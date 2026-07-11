# Совместимость tmux, передача клавиш и производительность рендера

## Контекст

Portalis запускает приложения в PTY с `TERM=xterm-256color`, но реализует только малую часть xterm/VT-последовательностей. Это особенно заметно в tmux, который активно использует редактирование строк, области прокрутки, DEC-режимы и ESC-последовательности.

Анализ кода выявил четыре независимые причины пользовательских симптомов.

### 1. Повреждение экрана tmux

`ansi.go` неверно обрабатывает `CSI M`: это команда удаления строк (DL), но сейчас она вызывает `ScrollUp()` и подписана как reverse line feed. Настоящий reverse index — `ESC M`.

Парсер не поглощает полностью последовательности выбора набора символов (`ESC ( B`, `ESC ( 0`, `ESC ) B`). После неизвестного `ESC (` последний байт (`B` или `0`) попадает на экран как обычный символ.

Не реализованы команды, которыми tmux обновляет части кадра:

- `CSI @` — вставить символы (ICH);
- `CSI P` — удалить символы (DCH);
- `CSI X` — стереть символы (ECH);
- `CSI L` — вставить строки (IL);
- `CSI M` — удалить строки (DL);
- `CSI S` / `CSI T` — прокрутить область вверх/вниз;
- `CSI d` — абсолютная вертикальная позиция курсора (VPA);
- `ESC 7` / `ESC 8` — сохранить/восстановить курсор;
- `ESC D` / `ESC E` / `ESC M` — index, next line, reverse index;
- `DECSET/DECRST ?1` — application cursor keys;
- `DECSET/DECRST ?25` — видимость курсора;
- `DECSET/DECRST ?2004` — bracketed paste.

### 2. Потеря хоткеев

`keyToBytes` перечисляет только `Ctrl+C`, `Ctrl+D`, `Ctrl+J`, `Ctrl+L`, `Ctrl+Z`. Bubble Tea кодирует `Ctrl+A`…`Ctrl+_` непосредственно значениями `KeyType` 0…31, но остальные значения сейчас возвращают `nil`. Поэтому стандартный tmux-prefix `Ctrl+B` не доходит до PTY.

Также не передаются F-клавиши и большинство модифицированных стрелок/Home/End/PageUp/PageDown.

`Ctrl+V` перехватывается как чтение системного clipboard, поэтому приложение внутри PTY не может использовать этот хоткей. Настоящая вставка терминала уже помечается Bubble Tea полем `KeyMsg.Paste` и не должна подменять `Ctrl+V`.

### 3. `Alt+Backspace`

Bubble Tea выдаёт `KeyMsg{Type: KeyBackspace, Alt: true}`. Portalis игнорирует `Alt` и отправляет только `DEL` (`0x7f`) вместо стандартной последовательности `ESC DEL` (`0x1b 0x7f`). Аналогично теряется `Alt` у rune/control/special keys.

### 4. Лаг и неверный размер

`Pty.Resize` при вызове чаще 50 мс записывает новый размер в `lastRows/lastCols`, но не вызывает `pty.Setsize`. Следующий вызов с тем же размером считается уже применённым и игнорируется. В результате tmux может продолжать рисовать под старый размер до конца сессии.

PTY читает по 4096 байт. Каждый chunk превращается в отдельный `PtyOutputMsg`, после которого Bubble Tea вызывает полный `Screen.Render()` по всей сетке. При большом выводе это создаёт сотни промежуточных кадров.

`Screen.Render()` повторно строит одинаковую строку даже если экран не изменился.

## Цель

Сделать Portalis совместимым с базовым интерактивным использованием tmux, корректно передавать клавиатурные последовательности xterm и сократить число лишних полных рендеров без заметной задержки ввода.

## Что изменится

1. `ansi.go` — корректная обработка ESC/CSI/DEC-последовательностей и режимов tmux.
2. `screen.go` — операции вставки/удаления символов и строк, reverse index, видимость курсора, application cursor mode, bracketed paste mode, dirty-cache рендера.
3. `emulator.go` — полный xterm encoder клавиш, передача Ctrl/Alt/F-key, прекращение перехвата `Ctrl+V`, bracketed paste по режиму экрана, объединение очереди PTY-output.
4. `pty.go` — корректный resize без потери последнего размера; единая реализация `Listen` без дублирования.
5. `ansi_test.go`, `screen_test.go`, `key_test.go`, новый `pty_test.go` — регрессионные сценарии.
6. `code-specs/ansi.md`, `code-specs/screen.md`, `code-specs/emulator.md`, `code-specs/pty.md`, `specs/portalis.md`, `README.md` — синхронизация документации.

## Детали реализации

### ANSI и экран

1. Расширить ESC-state так, чтобы последовательности с intermediate bytes поглощались целиком и не печатали designator на экран.
2. Реализовать `ESC 7`, `ESC 8`, `ESC D`, `ESC E`, `ESC M`.
3. Исправить `CSI M` на delete lines.
4. Добавить Screen-операции:
   - `InsertChars(n)`, `DeleteChars(n)`, `EraseChars(n)`;
   - `InsertLines(n)`, `DeleteLines(n)`;
   - `ScrollRegionUp(n)`, `ScrollRegionDown(n)`, `ReverseIndex()`.
5. Добавить обработку `CSI @`, `P`, `X`, `L`, `M`, `S`, `T`, `d`.
6. DEC-режимы обрабатывать для каждого параметра, а не только `params[0]`:
   - `?1` — application cursor keys;
   - `?25` — cursor visible;
   - `?1049` — alternate screen;
   - `?2004` — bracketed paste;
   - `?2026` — synchronized output.
7. Курсор рендерить только если одновременно `CursorVisible` и `CursorBlinkVisible`.
8. Добавить dirty-cache: неизменившийся Screen возвращает предыдущий render; любые мутации экрана/курсора/выделения/режима помечают его dirty.

### Клавиатура

1. Значения Bubble Tea `KeyType` 0…31 и 127 передавать как соответствующий control byte.
2. При `msg.Alt` добавлять xterm meta encoding:
   - rune/control/backspace → префикс `ESC`;
   - special keys → xterm modifier parameter.
3. Поддержать стрелки, Home/End, PageUp/PageDown, Insert/Delete, Shift/Ctrl/Alt combinations и F1–F20.
4. При application cursor mode базовые стрелки/Home/End кодировать через SS3 (`ESC O ...`).
5. Не перехватывать `Ctrl+V`: передавать `0x16` в PTY.
6. Для `KeyMsg.Paste` использовать `ESC[200~...ESC[201~`, только если дочернее приложение включило bracketed paste; иначе передавать текст напрямую.

### PTY и поток вывода

1. Удалить некорректный 50-мс resize throttle. `lastRows/lastCols` обновлять только после успешного `pty.Setsize`.
2. После первого PTY chunk неблокирующе собрать уже ожидающие chunks из `Output` в один `PtyOutputMsg` с ограничением максимального размера одного сообщения.
3. Использовать одну реализацию listener (`Pty.Listen`), удалить дублирующий `listenPty`.
4. Не добавлять искусственную задержку к первому chunk, чтобы интерактивный ввод оставался отзывчивым.

## Тесты

### Клавиши

Table-driven тест должен показать исходное Bubble Tea-событие и ожидаемые байты для:

- `Ctrl+B` → `0x02` (tmux prefix);
- `Ctrl+A`, `Ctrl+W`, `Ctrl+V`;
- `Alt+Backspace` → `ESC DEL`;
- `Alt+rune` → `ESC <utf8>`;
- обычные/Alt/Ctrl/Shift arrows;
- F1, F5, F12;
- application cursor arrow;
- bracketed и обычный paste.

### tmux/ANSI

Сценарные тесты «Дано / Когда / Тогда»:

1. `ESC(B` и `ESC(0` не оставляют `B`/`0` на экране.
2. `CSI M` удаляет строку внутри scroll region, а `ESC M` выполняет reverse index.
3. ICH/DCH/ECH изменяют только текущую строку.
4. IL/DL и SU/SD не выходят за scroll region.
5. `?25l/h` скрывает/показывает курсор.
6. Реалистичный tmux-frame из ESC/CSI-команд даёт ожидаемый plain-text snapshot.

### Производительность и resize

1. Два быстрых resize применяют последний физический размер PTY.
2. Listener объединяет заранее накопленные chunks и сохраняет порядок байтов.
3. Повторный `Render()` без мутаций возвращает cache; после изменения cache инвалидируется.
4. Benchmark сравнивает обработку tmux-frame и повторный cached render.

## Команды проверки

```bash
gofmt -w *.go
go test ./... -v
go test ./... -race
go test ./... -bench=. -benchmem
go vet ./...
```

Визуальный сценарий после unit-тестов:

1. Запустить минимальный Bubble Tea host с Portalis в PTY.
2. Внутри открыть tmux фиксированного размера.
3. Проверить статусную строку, resize, prefix `Ctrl+B`, стрелки, `Alt+Backspace`.
4. Сохранить `render.txt`, `state.json`, timeline и отчёт сценария.

## Критерии приёмки

- [ ] В tmux нет лишних `B`/`0` от charset escape sequences.
- [ ] Частичные обновления tmux через ICH/DCH/ECH/IL/DL/SU/SD отображаются корректно.
- [ ] `Ctrl+B` и остальные C0 control hotkeys доходят до PTY без потерь.
- [ ] `Ctrl+V` доходит до дочернего приложения; paste использует `KeyMsg.Paste`.
- [ ] `Alt+Backspace` отправляет `ESC DEL`.
- [ ] Модифицированные стрелки и F-клавиши имеют xterm-совместимые последовательности.
- [ ] Последний resize всегда физически применяется к PTY.
- [ ] Накопленные PTY chunks объединяются без изменения порядка.
- [ ] Неизменившийся экран не пересобирается на каждый `View`.
- [ ] Новые регрессионные и существующие тесты проходят с `-race`.
- [ ] `go vet ./...` проходит без ошибок.
- [ ] Per-file спецификации и README соответствуют реализации.
