# Specification: screen.go

## Назначение

`screen.go` реализует двумерную сетку терминального экрана (2D cell grid) с поддержкой:

- Отрисовки текста с атрибутами (цвет, стиль)
- Управления позицией курсора
- Обёртывания строк
- Скроллинга вверх (scrollback buffer)
- Выделения текста (selection)
- Синхронизированного вывода (synchronized output)
- Входов/выходов альтернативного экрана (ANSI SGR)

## Обзор типов

### `StyleBits`

```go
type StyleBits uint8
```

Флаговые типы стилей текста:

| Константа | Значение | Описание |
|-----------|----------|----------|
| `StyleBold` | 1 | Жирный шрифт |
| `StyleDim` | 2 | Тусклый шрифт |
| `StyleItalic` | 4 | Курсив |
| `StyleUnderline` | 8 | Подчёркивание |
| `StyleBlink` | 16 | Мигающий |
| `StyleReverse` | 32 | Инверсия (видео) |
| `StyleHidden` | 64 | Скрытый |
| `StyleStrikethrough` | 128 | Зачёркивание |

### `Cell`

```go
type Cell struct {
    Rune  rune      // Символ ячейки (0 = пробел)
    FG    lipgloss.Color // Цвет фона (foreground)
    BG    lipgloss.Color // Цвет текста (background)
    Style StyleBits   // Стилевые флаги
}
```

`Empty()` возвращает `true`, если ячейка пуста (Rune == 0 или пробел).

### `Cursor`

```go
type Cursor struct {
    Row int         // Строка (0-indexed)
    Col int         // Столбец (0-indexed)
    FG  lipgloss.Color
    BG  lipgloss.Color
    Style StyleBits
}
```

Курсор хранит позицию и атрибуты для отрисовки.

### `Screen`

```go
type Screen struct {
    Rows   int          // Высота экрана (количество строк)
    Cols   int          // Ширина экрана (количество столбцов)
    Cells  [][]Cell     // Двумерная сетка ячеек

    Cursor Cursor       // Текущая позиция курсора

    savedCursor Cursor        // Сохранённый курсор (ESC[?2026l)
    savedCells   [][]Cell      // Содержимое при входе в альтернативный экран

    scrollTop, scrollBottom int   // Границы области прокрутки (DECSTBM, 0-indexed)
    scrollback      [][]Cell   // Выведенные за пределы области строки
    scrollbackLimit int        // Лимит размера scrollback (по умолчанию 10000)
    viewOffset      int        // Смещение просмотра (сколько строк вверх от live-экрана)

    wrapPending bool  // true после записи в последнюю колонку; следующая буква оборачивает

    syncActive bool    // true во время синхронизированного вывода
    lastRender string   // Последний отрисованный кадр при sync

    CursorBlinkVisible bool  // Курсор мигает (для эмулятора)

    selStartRow, selStartCol int  // Начало выделения
    selEndRow, selEndCol     int   // Конец выделения
    selectionActive          bool // Активно ли выделение
}
```

## Публичный API

### Создание и инициализация

#### `NewScreen(rows, cols int) *Screen`

Создаёт новый экран с заданными размерами. Scrollback ограничивается `defaultScrollbackLimit` (10000 строк).

```go
s := NewScreen(24, 80)
```

#### `SetScrollbackLimit(limit int)`

Устанавливает максимальное количество строк в scrollback буфере.

- `0` или отрицательное значение — отключает лимит (не рекомендуется для долгих сессий)
- При увеличении лимита старые строки не удаляются
- При уменьшении лимита отбрасываются старые строки

### Управление курсором

#### `SetCursor(row, col int)`

Устанавливает позицию курсора.

- Индексы нормализуются: отрицательные → 0, ≥ Rows → Rows-1
- `wrapPending` сбрасывается

#### `CursorPos() (row, col int)`

Возвращает текущую позицию курсора.

#### `SaveCursor()`

Сохраняет текущую позицию курсора без атрибутов (FG/BG/Style сбрасываются).

#### `RestoreCursor()`

Восстанавливает сохранённую позицию курсора.

#### `Clear()`

Очищает весь экран. Сбрасывает `wrapPending`.

#### `ClearLine()`

Очищает строку курсора от курсора до конца строки.

#### `ClearLineLeft()`

Очищает строку курсора от начала до курсора (включительно).

#### `ClearLineAll()`

Очищает всю строку курсора.

### Запись текста

#### `Put(r rune)`

Записывает символ в позицию курсора и advances курсор.

**Поведение обёртывания:**

- Если запись в последнюю колонку → `wrapPending = true`
- Следующая запись автоматически переносит курсор на следующую строку
- При достижении верхней границы экрана строка поднимается в scrollback

#### `SetScrollRegion(top, bottom int)`

Устанавливает область прокрутки (DECSTBM).

- `top`, `bottom` — 1-indexed (как в ANSI)
- Нормализуется: `top < 1` → 1, `bottom > Rows` → Rows
- Если `bottom <= top` → `bottom = Rows`, `top = 1`
- `scrollTop`, `scrollBottom` конвертируются в 0-indexed

### Управление прокруткой

#### `ScrollUp()` / `scrollLineUp()`

Поднимает активную область на одну строку вверх.

**Побочные эффекты:**

- При `scrollTop == 0` первая строка сохраняется в `scrollback`
- При наличии активного выделения оно сбрасывается
- Scrollback буфер ограничен `scrollbackLimit`
- Нижняя строка очищается

#### `ScrollViewUp(n int)`

Перемещает просмотр вверх на `n` строк в scrollback буфер.

#### `ScrollViewDown(n int)`

Перемещает просмотр вниз (ближе к live-экрану).

#### `ResetView()`

Сбрасывает смещение просмотра к live-экрану.

#### `ViewOffset() int`

Возвращает текущее смещение просмотра.

### Альтернативный экран (ANSI SGR)

#### `EnterAltScreen()`

Входит в альтернативный экран:

- Сохраняет текущий курсор и содержимое экрана
- Очищает экран
- Устанавливает курсор в (0, 0)

#### `ExitAltScreen()`

Возвращается из альтернативного экрана:

- Восстанавливает сохранённый экран
- Восстанавливает сохранённый курсор
- Сбрасывает `wrapPending`

### Выделение текста

#### `StartSelection(row, col int)`

Начинает выделение с указанной ячейки.

#### `ExtendSelection(row, col int)`

Расширяет выделение до новой позиции.

**Правила:**

- Индексы нормализуются к границам экрана
- При перетаскивании влево по одной строке ячейка под мышью исключается из выделения

#### `ClearSelection()`

Сбрасывает активное выделение.

#### `cellInSelection(row, col int) bool`

Проверяет, находится ли ячейка внутри выделения.

#### `SelectionText() []string`

Возвращает текст текущего выделения.

- Каждая строка — отдельный элемент слайса
- Trailing whitespace обрезается
- При перетаскивании влево ячейка под мышью исключается

### Рендеринг

#### `Render() string`

Возвращает отрисованный экран как строку.

**Логика:**

- Если `syncActive` → возвращает `lastRender`
- Иначе обновляет и возвращает текущий кадр
- При `viewOffset > 0` строки берутся из scrollback буфера
- Иначе используется live-экран

#### `SetSync(active bool)`

Включает/выключает синхронизированный вывод.

- При `active = false` → сохраняет текущий кадр в `lastRender`
- При `active = true` → последующие вызовы `Render()` возвращают `lastRender`

### Утилитарные функции

#### `resize(rows, cols int)`

Внутренняя функция изменения размера экрана:

- Создаёт новую сетку ячеек
- Копирует существующий контент
- Сбрасывает `scrollTop`, `scrollBottom`
- Очищает кэш рендеринга

#### `LineText(row int) string`

Возвращает plain text строки экрана (без trailing пробелов).

Используется для захвата командной строки перед отправкой Enter в PTY.

## Реализация стилей

### `renderStyle(fg, bg lipgloss.Color, style StyleBits) string`

Конвертирует стиль в ANSI SGR код.

**Формат:**

```
\x1b[0;m  // сброс
[1;2;3;4;5;7;8;9;38;2;r;g;b;48;2;r;g;b]m
```

- `0` — сброс всех атрибутов
- `1` — Bold
- `2` — Dim
- `3` — Italic
- `4` — Underline
- `5` — Blink
- `7` — Reverse
- `8` — Hidden
- `9` — Strikethrough
- `38;2;r;g;b` — RGB foreground
- `48;2;r;g;b` — RGB background

### `sgrColor(c lipgloss.Color, bg bool) string`

Конвертирует `lipgloss.Color` в ANSI код:

- Hex цвета (`#RRGGBB`) → RGB SGR код
- Если цвет не hex → возвращает пустую строку

## Внутренние детали

### Scrollback Management

```
scrollback []Cell[Cols] — буфер выведенных строк
scrollbackLimit int    — лимит размера (по умолчанию 10000)
```

При добавлении новой строки:

1. Копируется содержимое верхней строки в scrollback
2. Если `len(scrollback) > scrollbackLimit` → обрезка старых строк

### Selection Logic

```
selStartRow, selStartCol — начало выделения
selEndRow, selEndCol     — конец выделения
selectionActive          — флаг активности
```

При перетаскивании влево по одной строке:

```
exclusiveStart = (startRow == endRow && startCol > endCol)
// ячейка под мышью (меньший col) исключается из выделения
```

### Render Optimization

Рендеринг оптимизирован для минимизации ANSI эскэпов:

- Стили применяются только при изменении
- Курсор/выделение получают `\x1b[7m` (reverse) + `\x1b[0m`
- В конце строки — сброс стилей

## Ограничения

1. **Scrollback limit** по умолчанию 10000 строк
2. **Cursor** всегда в пределах `0..Rows-1` × `0..Cols-1`
3. **Selection** работает только когда `selectionActive = true`
4. **Synchronized output** требует явного включения через `SetSync(true)`
5. **Hex цвета** поддерживаются только формат `#RRGGBB`

## Примеры использования

### Создание экрана

```go
s := NewScreen(24, 80)
s.SetCursor(0, 0)
s.Put('H')
```

### Управление прокруткой

```go
s.Put('X') // ... много Put() ...
s.ScrollUp() // строка поднимается в scrollback
```

### Выделение текста

```go
s.StartSelection(10, 5)
s.ExtendSelection(10, 15)
text := s.SelectionText() // ["X X X X X X X X X X X X X X X X"]
```

### Альтернативный экран

```go
s.EnterAltScreen()
s.Clear()
// отрисовка альтернативного экрана
s.ExitAltScreen() // восстановление
```

### Синхронизированный вывод

```go
s.SetSync(true)
// любые изменения не влияют на отображение
s.SetSync(false) // восстановление последнего кадра
```
