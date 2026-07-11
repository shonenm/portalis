# Спецификация проекта Portalis

## Обзор

**Portalis** — встроенный терминальный эмулятор для правой панели приложения Automata. Предоставляет полноценную терминальную сессию с поддержкой ANSI/VT-последовательностей, PTY, буфером обмена и интеграцией с Bubble Tea.

## Архитектура

### Компоненты

```
portalis/
├── ansi.go          # ANSI-парсер: CSI, OSC, UTF-8, цвета
├── clipboard.go     # Работа с буфером обмена (macOS/Linux/Wayland)
├── emulator.go      # Terminal emulator (координирует Screen + Parser + PTY)
├── pty.go           # PTY обёртка (spawn, write, read, resize)
└── screen.go        # Экран: 2D сетка, рендеринг, scrollback, выделение
```

### Диаграмма компонентов

```
┌─────────────────────────────────────────────────────────────────┐
│                        Emulator (Главный контроллер)              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │   Parser     │→ │    Screen    │←│      PTY              │  │
│  │ (ANSI parser)│  │ (rendering)  │  │ (PTY process I/O)    │  │
│  └──────────────┘  └──────────────┘  └──────────────────────┘  │
│         ↑                  ↑                  ↑                  │
│         │                  │                  │                  │
│   ANSI sequences      User input          PTY output            │
│   (CSI, OSC)          (keyboard/mouse)    (stdout/stderr)       │
└─────────────────────────────────────────────────────────────────┘
```

### Emulator implements warp.Panel

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
    width     int             // Ширина панели (от warp)
    height    int             // Высота панели (от warp)
    stopped   bool            // Завершена ли сессия
    cwd       string          // Последняя рабочая директория
    commandHistory []string      // История команд (max 1000)
    initialCWD string          // Изначальная рабочая директория
    scrollbackLimit int         // Лимит скролла (по умолчанию 10000)
    pressX, pressY int         // Позиция нажатия мыши
    dragSelecting bool        // В процессе ли drag-выбора
    mu        sync.RWMutex   // Мьютекс для синхронизации
}
```

## Жизненный цикл сессии

### Запуск

```
1. Пользователь выбирает чат/терминал в дереве
2. SessionManager.openItem создаёт Emulator
3. App.Update(ItemSelectedMsg) вызывает em.Start()
4. em.Start() спавнит PTY процесс
5. Возвращается PtyReadyMsg
```

### Обновление

```
1. Emulator.Update(msg) обрабатывает:
   - tea.KeyMsg → запись в PTY, парсинг ANSI
   - tea.MouseMsg → SGR мышь, скроллинг, drag-выбор
   - tea.WindowSizeMsg → игнорируется (resize через ResizeMsg)
   - ResizeMsg → обновление размеров экрана и PTY
   - PtyOutputMsg → парсинг вывода через Parser
   - PtyExitMsg → завершение сессии
   - CursorBlinkMsg → переключение мигания курсора
```

### Завершение

```
1. При EOF от PTY приходит PtyExitMsg
2. SessionManager удаляет сессию
3. Emulator.Stop() закрывает PTY
4. Panel показывает ASCII-арт вместо терминала
```

## Данные потока

### Входные данные

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Keyboard Event  │────▶│     Emulator    │────▶│      Parser     │
│  (KeyMsg/Mouse)  │     │  (Event Loop)   │     │   (ANSI States) │
└─────────────────┘     └────────┬─────────┘     └────────┬─────────┘
                                 │                         │
                                 ▼                         ▼
                         ┌─────────────────┐     ┌─────────────────┐
                         │   Screen       │     │   Screen.Put()  │
                         │  (Cell Grid)   │◀────│  (Update Buffer) │
                         └─────────────────┘     └─────────────────┘
                                 ▲                         ▲
                                 │                         │
                                 └─────────────────────────┘
                                         Render String
```

### Выводные данные

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│      PTY        │────▶│    Screen       │────▶│    Render()     │
│  (Read Loop)    │     │  (Cell Grid)    │     │  (String)       │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

### ANSI Парсинг

```
Input Data → Parser.Feed() → CSI/OSC/SGR → Screen.Put() → Render
```

## Публичный API

### Emulator

| Метод | Описание |
|-------|----------|
| `NewEmulator(sessionID, chatName, cmd, args)` | Создание эмулятора |
| `Start()` / `StartWithEnv()` | Запуск PTY |
| `StartSync(extraEnv)` | Синхронный запуск |
| `View(width, height)` | Рендеринг экрана |
| `Update(msg)` | Обработка событий |
| `Focus()` / `Blur()` | Управление фокусом |
| `Close()` | Закрытие PTY |
| `Stop()` | Остановка сессии |
| `SetScrollbackLimit(limit)` | Настройка лимита скролла |
| `SetInitialCWD(dir)` | Установить начальную директорию |
| `SetCommandHistory(history)` | Восстановить историю команд |

### Callbacks

| Callback | Описание |
|----------|----------|
| `OnCWDChange(fn func(string))` | Вызывается при смене рабочей директории |
| `OnCommandHistoryChanged(fn func([]string))` | Вызывается при изменении истории команд |

### Screen

| Метод | Описание |
|-------|----------|
| `NewScreen(rows, cols)` | Создание экрана |
| `Put(r rune)` | Запись символа |
| `SetCursor(row, col)` | Установка курсора |
| `ScrollUp()` / `ScrollViewUp(n)` | Скроллинг вверх |
| `Clear()`, `ClearLine()`, `ClearLineLeft()` / `ClearLineAll()` | Очистка |
| `EnterAltScreen()` / `ExitAltScreen()` | Альтернативный экран |
| `StartSelection(row, col)` / `ExtendSelection(row, col)` / `ClearSelection()` | Выделение |
| `SelectionText()` | Получение выделенного текста |
| `Render()` | Рендеринг экрана |

### Parser

| Метод | Описание |
|-------|----------|
| `NewParser(screen)` | Создание парсера |
| `Feed(data []byte)` | Парсинг ANSI данных |
| `SetCWDCallback(fn)` | Callback для изменений рабочей директории |

### PTY

| Метод | Описание |
|-------|----------|
| `Spawn(command, args, env...)` | Запуск PTY |
| `SpawnInDir(command, args, dir, env...)` | Запуск в директории |
| `Write(data)` | Отправка данных в PTY |
| `Resize(rows, cols)` | Изменение размера |
| `Close()` | Закрытие PTY |
| `Listen(sessionID)` | Слушатель событий |

### Clipboard

| Функция | Описание |
|---------|----------|
| `copyToClipboard(lines)` | Копирование в буфер обмена |
| `pasteFromClipboard()` | Вставка из буфера обмена |

## Дизайн-решения

### 1. Совместимость с tmux и xterm

- Поддержка наборов символов DEC Special Graphics (`ESC(B`, `ESC(0`) и управляющих ESC-последовательностей (`ESC 7`, `ESC 8`, `ESC D`, `ESC E`, `ESC M`).
- Редактирование строк: `ICH` (`CSI @`), `DCH` (`CSI P`), `ECH` (`CSI X`), `IL` (`CSI L`), `DL` (`CSI M`).
- Прокрутка внутри области: `SU` (`CSI S`) / `SD` (`CSI T`) и `ReverseIndex`.
- Режимы: application cursor (`?1`), видимость курсора (`?25`), bracketed paste (`?2004`), synchronized output (`?2026`), alternate screen (`?1049`).
- VPA (`CSI d`) и HPA (`CSI G`).

### 2. Модель событий (Bubble Tea)

Emulator использует Bubble Tea для обработки событий:
- Event loop через `Update(msg tea.Msg) tea.Cmd`
- Асинхронные операции через `tea.Cmd`
- Сообщения через `tea.Msg`

### 2. Состояние и синхронизация

- `sync.RWMutex` защищает доступ к мутабельному состоянию
- Parser не держит mutex во время парсинга (возвращает parser без mutex)
- Callbackы из Parser могут вызывать методы Emulator (требуется синхронизация)

### 3. Scrollback Management

- Scrollback буфер ограничивается `scrollbackLimit` (по умолчанию 10000)
- При заполнении старые строки удаляются
- View offset позволяет "прокручивать" scrollback буфер

### 4. Bracketed Paste

- Текст вставляется через `ESC[200~...ESC[201~`, только если дочернее приложение включило режим `?2004`.
- Вставка через Bubble Tea `KeyMsg.Paste` передаётся как plain text без режима или обёрнутой в bracketed, в зависимости от состояния экрана.
- Изображения передаются через путь к файлу.
- Поддержка macOS (Swift), Wayland, X11.

### 5. OSC 7 для рабочей директории

- PTY конфигурируется с `PROMPT_COMMAND` для эмитирования OSC 7
- Callback вызывается при изменении cwd
- Поддержка форматов `file://hostname/path` и `/absolute/path`

### 6. Synchronized Output

- `ESC[?2026h` (вход) / `ESC[?2026l` (выход)
- Во время sync промежуточные изменения не отображаются
- `Render()` возвращает последний закоммиченный кадр
- Применяется внутри `Screen.SetSync`, а не вручную

### 7. Управление курсором

- `ESC[?1h` / `ESC[?1l` — application cursor keys
- `ESC[?25h` / `ESC[?25l` — видимость курсора
- Сохранение/восстановление позиции без атрибутов (`ESC 7` / `ESC 8`)
- Синхронизированное мигание курсора через `CursorBlinkMsg`

### 8. Передача клавиш в PTY

- Все C0 control keys (Ctrl+A…Ctrl+_, Ctrl+Space, Backspace) передаются как соответствующие байты 0x00–0x1F/0x7F.
- Модификатор `Alt` добавляет префикс `ESC` для rune/backspace/control; для стрелок, Home/End, PageUp/PageDown, F1–F20 используется xterm-параметр modifier (1;2/3/5/7/8).
- При application cursor mode базовые стрелки и Home/End кодируются через SS3 (`ESC O ...`).
- `Ctrl+V` не перехватывается: передаётся в PTY как 0x16, а clipboard-вставка идёт через `KeyMsg.Paste`.

### 9. Размер PTY и производительность

- `Pty.Resize` применяет каждый размер напрямую через `pty.Setsize`, без потерь из-за throttle.
- `Pty.Listen` при наличии нескольких queued chunks объединяет их в один `PtyOutputMsg` до 64 KiB, чтобы избежать лишних полных рендеров.
- `Screen` использует dirty cache: неизменившийся кадр возвращает предыдущий render, инвалидация происходит при любом изменении ячеек/курсора/выделения/режимов.

## Ограничения

1. **Scrollback limit** — по умолчанию 10000 строк
2. **Command history** — макс 1000 команд
3. **Cursor** — всегда в пределах границ экрана
4. **Selection** — работает только когда `selectionActive = true`
5. **Hex цвета** — поддерживается только формат `#RRGGBB`

## Безопасность

1. Все ошибки обрабатываются явно (не игнорируются)
2. Проверка на закрытый PTY перед операциями
3. Процесс убивается при закрытии PTY
4. Временные файлы используют UUID-имена
5. Изображения перекодируются для чистоты данных

## Зависимости

- `github.com/charmbracelet/bubbletea` — TUI фреймворк
- `github.com/charmbracelet/lipgloss` — стилизация текста
- `github.com/creack/pty` — PTY поддержка

## Примечания

- Код работает только на платформах с соответствующими инструментами
- Изображения сохраняются во временную директорию и должны быть удалены
- Swift требуется для работы с изображениями на macOS
- PTY использует `xterm-256color` терминал
- Вывод обрабатывается через `bufio.Reader` для корректного разбора

## Связанные спецификации

- `code-specs/ansi.md` — детальная спецификация Parser
- `code-specs/screen.md` — детальная спецификация Screen
- `code-specs/pty.md` — детальная спецификация PTY
- `code-specs/emulator.md` — детальная спецификация Emulator
- `code-specs/clipboard.md` — детальная спецификация Clipboard
