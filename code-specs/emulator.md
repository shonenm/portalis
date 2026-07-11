# Emulator

## Назначение

`Emulator` — встроенный терминальный эмулятор, который запускает оболочку/command в PTY (псевдо-терминал) и отображает вывод. Работает независимо от UI-фреймворка; хосты подают ему сообщения о клавиатуре/мышь/размере и вызывают `View(width, height)` для рендеринга.

Использует библиотеку `bubbletea` для управления TUI-приложением.

## Публичный API

### Конструктор

```go
func NewEmulator(sessionID, chatName, command string, args []string) *Emulator
```

Создаёт новый терминальный эмулятор для указанной сессии. Если `command` пуст, пытается найти оболочку (bash, sh).

| Параметр | Тип | Описание |
|----------|-----|----------|
| sessionID | string | Идентификатор сессии |
| chatName | string | Имя чата |
| command | string | Команда для запуска (оболочка) |
| args | []string | Аргументы команды |

### Старт

```go
func (e *Emulator) Start() tea.Cmd
```

Начинает запуск процесса PTY. Возвращает `tea.Cmd`, который возвращит `PtyReadyMsg` по готовности.

```go
func (e *Emulator) StartSync(extraEnv []string) error
```

Запускает PTY синхронно с дополнительными переменными окружения. Не возвращает `tea.Cmd` — PTY готов сразу. Возвращает ошибку при неудаче.

```go
func (e *Emulator) StartWithEnv(extraEnv []string) tea.Cmd
```

Запуск PTY с переменными окружения. Возвращает `tea.Cmd`, завершающий работу и возвращающий `PtyReadyMsg` или `PtyExitMsg`.

```go
func (e *Emulator) SetScrollbackLimit(limit int)
```

Устанавливает максимальное количество линий скролла. Значения ≤ 0 отключают лимит. Вызывать до `Start()` для вступления в силу, после `Start()` обновляет экран сразу.

```go
func (e *Emulator) SetInitialCWD(dir string)
```

Устанавливает каталог, в котором начнётся PTY-процесс.

```go
func (e *Emulator) SetCommandHistory(history []string)
```

Восстанавливает ранее сохранённую историю команд.

```go
func (e *Emulator) Close()
```

Закрывает PTY.

```go
func (e *Emulator) Stop()
```

Завершает сессию и переключает панель на вид с ASCII-артом.

```go
func (e *Emulator) Focus()
```

Отмечает эмулятор как сфокусированный.

```go
func (e *Emulator) Blur()
```

Отмечает эмулятор как нефокусированный.

```go
func (e *Emulator) View(width, height int) string
```

Рендерит терминальный экран при заданном размере панели. Размер экрана синхронизируется через `ResizeMsg`.

```go
func (e *Emulator) Update(msg tea.Msg) tea.Cmd
```

Обрабатывает сообщения.

### Callbacks

```go
func (e *Emulator) OnCWDChange func(string)
```

Вызывается при смене рабочей директории.

```go
func (e *Emulator) OnCommandHistoryChanged func([]string)
```

Вызывается при изменении истории команд.

### Debug

```go
func (e *Emulator) Pty() *Pty
```

Возвращает underlying PTY для отладки.

## Типы сообщений

### ResizeMsg

```go
type ResizeMsg struct {
    Width  int
    Height int
}
```

Подаётся хостом, когда выделенная прямоугольная область эмулятора изменилась. Содержит размер контента в ячейках (без границ или отступов).

### CursorBlinkMsg

```go
type CursorBlinkMsg struct{}
```

Подаётся хостом для переключения состояния мигания курсора. Один таймер хоста должен транслировать это сообщение всем видимым терминальным эмуляторам для синхронизации мигания курсоров.

### PtyReadyMsg

```go
type PtyReadyMsg struct {
    SessionID      string
    AlreadyRunning bool
}
```

Подаётся, когда PTY готов к прослушиванию.

### PtyExitMsg

```go
type PtyExitMsg struct {
    SessionID string
    Err       error
}
```

Подаётся, когда PTY завершил работу с ошибкой.

## Структура данных Emulator

```go
type Emulator struct {
    SessionID string          // Идентификатор сессии
    ChatName  string          // Имя чата/сессии
    cmd       string          // Команда для запуска (bash/sh)
    args      []string        // Аргументы команды
    screen    *Screen         // Экран терминала
    parser    *Parser         // Парсер вывода
    pty       *Pty            // PTY процесс
    focused   bool            // Сфокусирован или нет
    width     int             // Ширина панели
    height    int             // Высота панели
    stopped   bool            // Завершена ли сессия
    cwd       string          // Последняя рабочая директория (OSC 7)
    commandHistory []string       // История команд (max 1000)
    initialCWD string          // Изначальная рабочая директория
    scrollbackLimit int          // Лимит скролла
    pressX, pressY int         // Позиция нажатия мыши
    dragSelecting bool          // В процессе ли drag-выбора
    mu        sync.RWMutex   // Мьютекс для синхронизации
}
```

## Вспомогательные функции

### DefaultShell / defaultShell

```go
func DefaultShell() (string, []string)
func defaultShell() (string, []string)
```

Возвращают рабочую оболочку (bash, sh). `defaultShell` — устаревший внутренний алиас для обратной совместимости.

### stripPrompt

```go
func stripPrompt(line string) string
```

Удаляет префикс shell prompt с терминальной строки. Ищет последнее появление общих маркеров завершения prompt: `$ `, `# `, `> `, `% `.

### renderAsciiArt

```go
func renderAsciiArt(width, height int) string
```

Возвращает ASCII-арт иконку, показываемую, когда сессия завершена. Центрирует арт в заданном размере.

### emptyView

```go
func emptyView(width, height int) string
```

Возвращает пустой экран (пробелы) для эмулятора, у которого ещё нет PTY.

### keyToBytes / keyToBytesWithModes

```go
func keyToBytes(msg tea.KeyMsg) []byte
func keyToBytesWithModes(msg tea.KeyMsg, modes keyEncodingModes) []byte
```

Конвертирует `tea.KeyMsg` в последовательность байтов, отправляемую PTY. Учитывает режимы экрана (`applicationCursor`, `bracketedPaste`).

**Поддерживаемые группы:**
- C0 control keys: `Ctrl+A`…`Ctrl+_`, `Ctrl+Space`, `Backspace` — передаются как соответствующие байты 0x00–0x1F/0x7F
- Rune keys и paste — plain bytes, с `ESC` префиксом при `Alt`
- Bracketed paste — если `KeyMsg.Paste` и режим `?2004` включён, оборачивается в `ESC[200~...ESC[201~`
- Стрелки, Home/End, PageUp/PageDown — xterm CSI/SS3 с modifier-параметром (1;2/3/5/7/8); SS3 используется при application cursor mode
- F1–F20 — xterm/urxvt sequences
- `Shift+Tab` — `ESC[Z`
- `Ctrl+V` — передаётся как 0x16; clipboard-вставка обрабатывается через `KeyMsg.Paste`

### mouseToBytes

```go
func mouseToBytes(msg tea.MouseMsg) []byte
```

Кодирует событие мыши bubbletea в SGR-последовательность мыши, чтобы TUI-приложения внутри PTY получали отдельные события press/release/wheel.

## Правила

- Терминальный рендеринг должен сохранять keyboard-first взаимодействие.
- Поддержка мыши разрешена, но она не должна быть единственной моделью взаимодействия без явного решения.
- Запуск дочерних процессов должен иметь явные границы и обработку ошибок.

## Внутреннее поведение

### Старт

1. Создаётся `Screen` 24×80 (можно изменить через `SetScrollbackLimit`).
2. Создаётся `Parser` с callback'ом на смену рабочей директории.
3. Запускается PTY через `spawnPty(extraEnv)`.
4. Если ширина/высота заданы — ресайз экрана и PTY.

### Обработка клавиш

- `Ctrl+V` передаётся в PTY как 0x16; clipboard-вставка идёт через `KeyMsg.Paste`.
- Любое нажатие возвращает экран в live режим.
- При `Enter` извлекается команда из строки, сохраняется в историю (max 1000).
- Байты пишутся синхронно в PTY для сохранения порядка нажатий.
- Модификаторы `Alt`/`Ctrl`/`Shift` и F-клавиши кодируются в xterm-совместимые последовательности.
- При application cursor mode (`DECSET ?1`) базовые стрелки и Home/End кодируются через SS3 (`ESC O ...`).

### Обработка мыши

- Колёсо: скроллинг вверх/вниз на 3 линии.
- Левая кнопка:
  - Press → запоминаем позицию.
  - Motion > 1 клетка → начинаем drag-выбор.
  - Release → копируем выделенный текст в буфер обмена.
- Все события (включая Press) форвардятся в PTY, чтобы приложения получали полные кликовые последовательности.

### Обработка ресайза

- `WindowSizeMsg` от bubbletea игнорируется — размер панели идёт через `ResizeMsg`.
- `ResizeMsg` обновляет `width`, `height`, ресайзит экран и PTY.
- PTY применяет каждый уникальный размер напрямую через `pty.Setsize` без потерь из-за throttle.

### Скроллинг

- `scrollUp(lines)` / `scrollDown(lines)` — скролл вверх/вниз.
- Колёсо мыши вызывает `scrollUp(3)` / `scrollDown(3)`.
