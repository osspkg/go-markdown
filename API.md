# API справочник

Пакет `go.osspkg.com/markdown` предоставляет `Converter` для разбора Markdown
в дерево токенов и рендеринга этого дерева в HTML, а также для преобразования
HTML-документов и фрагментов в GFM Markdown. В таблицах ниже перечислены
экспортируемые методы, параметры настройки и встроенные идентификаторы токенов
с атрибутами.

## Основные методы

| Метод | Что делает |
| --- | --- |
| `markdown.New(opts ...markdown.Option) (*markdown.Converter, error)` | Создаёт конвертер со встроенным парсером и HTML-обработчиками. По умолчанию использует компактный HTML, режим fail-fast, лимит входа 16 МиБ, глубину 128 и 1 000 000 токенов. |
| `(*markdown.Converter).Convert(source string) (markdown.ConversionResult, error)` | Разбирает Markdown и сразу возвращает готовый HTML вместе с предупреждениями. |
| `(*markdown.Converter).ConvertHTML(source string) (string, error)` | Разбирает документ или фрагмент HTML5-парсером, преобразует содержимое `<body>` в GFM Markdown и возвращает ошибку при неверном UTF-8 или превышении лимитов. Сущности декодируются, некорректная разметка восстанавливается. Первые `<title>` и `meta[name=description]` из `<head>` переносятся в YAML frontmatter, если их значения непустые. |
| `(*markdown.Converter).Tokenize(source string) (markdown.TokenizationResult, error)` | Разбирает Markdown и возвращает дерево токенов без рендеринга. Переводы строк нормализуются к LF. |
| `(*markdown.Converter).Render(w io.Writer, tokens []markdown.Token) ([]markdown.Diagnostic, error)` | Рендерит переданное дерево токенов в `io.Writer`. Токены можно получить через `Tokenize` или создать самостоятельно. |
| `(*markdown.Converter).RegisterHandler(id markdown.TokenID, handler markdown.Handler) error` | Регистрирует или заменяет обработчик рендеринга для идентификатора токена. Последняя регистрация имеет приоритет. |
| `(*markdown.Converter).RegisterRule(rule markdown.Rule) error` | Добавляет пользовательское правило разбора на уровне блока или inline-текста. |
| `(*markdown.Converter).RegisterHTMLHandler(tag string, handler markdown.HTMLHandler) error` | Регистрирует или заменяет обработчик HTML-тега при преобразовании HTML в Markdown. Имя тега приводится к нижнему регистру; последняя регистрация выигрывает. |
| `(*markdown.Converter).RegisterHTMLRule(rule markdown.HTMLRule) error` | Добавляет правило сопоставления HTML-элементов для преобразования в Markdown. Сначала выполняются правила с большим приоритетом; при равенстве выигрывает последняя регистрация. Правило применяется раньше обработчика, зарегистрированного по имени тега. |

Настраивайте конвертер и регистрируйте правила и обработчики до первого вызова
`Convert`, `ConvertHTML`, `Tokenize` или `Render`. После первого вызова
настройки заморожены; дальше конвертер можно использовать конкурентно.
HTML-обработчики и правила не запускаются для содержимого `<script>` и
`<style>`: эти элементы всегда пропускаются.

## Методы контекста и курсора

| Метод | Что делает |
| --- | --- |
| `(*markdown.RenderContext).RenderChildren(w io.Writer, token markdown.Token) error` | Рендерит дочерние токены по порядку. |
| `(*markdown.RenderContext).WriteText(w io.Writer, value string) error` | Декодирует HTML-ссылки и экранирует текст для безопасного вывода. |
| `(*markdown.RenderContext).WriteAttribute(w io.Writer, value string) error` | Экранирует значение HTML-атрибута. |
| `(*markdown.RenderContext).SafeURL(value string) (string, bool)` | Проверяет URL по политике встроенного рендерера. Разрешены `http`, `https`, `mailto`, `tel`, `xmpp`, относительные URL и фрагменты. |
| `(*markdown.RenderContext).Source(token markdown.Token) string` | Возвращает часть исходного текста по `Span`. Доступна при вызове из `Convert`; при отдельном вызове `Render` возвращается пустая строка. |
| `(*markdown.HTMLRenderContext).RenderChildren(w io.Writer, node markdown.HTMLNode) error` | Рендерит дочерние HTML-узлы в текущем блочном или inline-режиме; дочерние узлы также проходят через зарегистрированные HTML-правила и обработчики. |
| `(*markdown.HTMLRenderContext).WriteText(w io.Writer, value string) error` | Экранирует Markdown-пунктуацию для буквального вывода текста. |
| `(markdown.HTMLNode).Tag() string` | Возвращает имя HTML-тега в нижнем регистре. Для текстового узла возвращает пустую строку. |
| `(markdown.HTMLNode).Text() string` | Возвращает объединённый текст узла и его потомков. |
| `(markdown.HTMLNode).Attribute(name string) (string, bool)` | Возвращает значение атрибута без учёта регистра имени и признак его наличия. |
| `(*markdown.ParseContext).Source() string` | Возвращает нормализованный UTF-8 исходный текст, доступный правилу разбора. |
| `(*markdown.ParseContext).Phase() markdown.RulePhase` | Возвращает фазу текущего пользовательского правила. |
| `(*markdown.ParseContext).ParseInline(start, end int) ([]markdown.Token, error)` | Разбирает диапазон исходного текста как inline Markdown. Диапазон задаётся полуоткрытыми байтовыми смещениями. |
| `(*markdown.ParseContext).ParseBlocks(start, end int) ([]markdown.Token, error)` | Разбирает диапазон исходного текста как блоки Markdown. Диапазон задаётся полуоткрытыми байтовыми смещениями. |
| `(markdown.Cursor).Position() int` | Возвращает текущее байтовое смещение курсора. |
| `(markdown.Cursor).Remaining() string` | Возвращает неразобранную часть доступного диапазона. |
| `(markdown.Cursor).AtEnd() bool` | Проверяет, остался ли текст для разбора. |
| `(markdown.Cursor).Peek() (rune, bool)` | Читает следующую Unicode-кодовую точку без перемещения курсора. Второе значение показывает, удалось ли прочитать символ. |
| `(markdown.Cursor).Slice(size int) (string, bool)` | Возвращает первые `size` байт без перемещения курсора. Не допускает выход за диапазон или разрезание UTF-8 символа. |
| `(markdown.Cursor).Source() string` | Возвращает весь исходный текст, доступный курсору. |
| `(markdown.Diagnostic).Error() string` | Возвращает текст предупреждения. Метод реализует `error`. |
| `(markdown.Diagnostic).Unwrap() error` | Возвращает исходную ошибку предупреждения для `errors.Is` и `errors.As`. |
| `(*markdown.ParseError).Error() string` | Возвращает ошибку разбора или рендеринга с фазой и диапазоном исходного текста. Метод реализует `error`. |
| `(*markdown.ParseError).Unwrap() error` | Возвращает исходную ошибку для `errors.Is` и `errors.As`. |

## Параметры конструктора

Передавайте эти функции в `markdown.New(...)`. Недопустимое значение возвращает
ошибку при создании конвертера.

| Опция | Значение |
| --- | --- |
| `markdown.WithErrorMode(mode)` | Для разбора и рендеринга Markdown `ErrorModeFailFast` останавливает обработку на первой фатальной ошибке, а `ErrorModeBestEffort` записывает восстанавливаемые ошибки в `Warnings`. `ConvertHTML` возвращает ошибки напрямую. По умолчанию — `ErrorModeFailFast`. |
| `markdown.WithHTMLFormat(format)` | `HTMLFormatCompact` выдаёт компактный HTML. `HTMLFormatPretty` добавляет отступы и переводы строк. По умолчанию — `HTMLFormatCompact`. |
| `markdown.WithUnsafeRawHTML()` | Разрешает выводить raw HTML, но встроенный рендерер всё равно фильтрует запрещённые GFM-теги. Включайте только для доверенного Markdown. |
| `markdown.WithMaxInputBytes(limit)` | Ограничивает размер нормализованного входа в байтах. По умолчанию — 16 МиБ; значение `0` отключает лимит. Отрицательное значение недопустимо. |
| `markdown.WithMaxNestingDepth(limit)` | Ограничивает глубину разбора и рендеринга. По умолчанию — 128; значение должно быть положительным. |
| `markdown.WithMaxTokens(limit)` | Ограничивает число токенов Markdown или узлов HTML при одном разборе. По умолчанию — 1 000 000; значение должно быть положительным. |

### Значения перечислений

| Тип | Значение | Описание |
| --- | --- | --- |
| `RulePhase` | `RulePhaseUnknown` | Неуказанная фаза; не подходит для регистрации правила. |
| `RulePhase` | `RulePhaseBlock` | Правило разбирает блоки Markdown. |
| `RulePhase` | `RulePhaseInline` | Правило разбирает inline-текст. |
| `RulePhase` | `RulePhaseRender` | Фаза для диагностик рендеринга; пользовательские правила в этой фазе не регистрируются. |
| `ErrorMode` | `ErrorModeUnknown` | Неуказанный режим; не подходит для настройки конвертера. |
| `ErrorMode` | `ErrorModeFailFast` | Возвращать первую фатальную ошибку. Используется по умолчанию. |
| `ErrorMode` | `ErrorModeBestEffort` | Записывать восстанавливаемые ошибки в `Warnings` и продолжать обработку. |
| `HTMLFormat` | `HTMLFormatUnknown` | Неуказанный формат; не подходит для настройки конвертера. |
| `HTMLFormat` | `HTMLFormatCompact` | Компактный детерминированный HTML. Используется по умолчанию. |
| `HTMLFormat` | `HTMLFormatPretty` | Детерминированный HTML с отступами и переносами строк. |

## Типы для работы с токенами и расширениями

| Тип | Поля или назначение |
| --- | --- |
| `TokenID` | Строковый идентификатор токена. Можно объявить собственный идентификатор для пользовательских правил и обработчиков. |
| `Span` | `Start` и `End` — полуоткрытые байтовые смещения в нормализованном UTF-8 источнике. |
| `Token` | `ID` — вид токена; `Literal` — буквальное содержимое; `Attributes` — строковые атрибуты; `Children` — дочерние токены; `Span` — положение в источнике. Обработчики должны считать токен доступным только для чтения. |
| `Rule` | `ID` — ID создаваемого токена; `Phase` — `RulePhaseBlock` или `RulePhaseInline`; `Priority` — порядок запуска; `Parse` — функция разбора. При совпадении функция возвращает токен, количество потреблённых байтов, `matched=true` и ошибку. Без совпадения возвращает `matched=false` и `consumed=0`. |
| `Handler` | Функция, которая принимает `*RenderContext`, `io.Writer` и токен, записывает HTML и возвращает ошибку. |
| `HTMLHandler` | Функция, которая принимает `*HTMLRenderContext`, `io.Writer` и `HTMLNode`, записывает Markdown и возвращает ошибку. |
| `HTMLRule` | Поля `ID`, `Priority`, `Match` и `Handler` задают приоритетное правило рендеринга HTML-элементов. `Match` получает read-only представление `HTMLNode`; совпавшее правило выполняется раньше обработчика тега. |
| `HTMLNode` | Read-only представление HTML-узла. Предоставляет имя тега, текст потомков и значения атрибутов через методы `Tag`, `Text` и `Attribute`. |
| `HTMLRenderContext` | Контекст HTML-обработчика. Рендерит дочерние узлы с тем же режимом вложенности и экранирует Markdown-текст. |
| `Option` | Функция настройки, принимаемая `New`. Публичные опции перечислены выше. |
| `ParseContext` | Контекст пользовательского правила с доступом к исходнику, фазе и рекурсивному разбору диапазонов. |
| `Cursor` | Только для чтения показывает текущую позицию и остаток входа пользовательского правила. Потреблённые байты возвращаются из `Rule.Parse`, сам курсор не перемещается. |
| `RenderContext` | Доступен обработчику на время его вызова; предоставляет рендеринг дочерних токенов, экранирование, проверку URL и доступ к фрагменту источника. |

## Встроенные токены

`Token.ID` содержит идентификатор токена. `Token.Children` хранит дочерние узлы,
`Token.Literal` — текстовое содержимое, а `Token.Attributes` — строковые
атрибуты. `Span` задаёт полуоткрытый диапазон байтов в нормализованном UTF-8
тексте. У структурных токенов содержимое обычно находится в `Children`.

| Константа | ID | Человекопонятное описание | Атрибуты |
| --- | --- | --- | --- |
| `TokenDocument` | `document` | Корневой узел всего документа. | — |
| `TokenParagraph` | `paragraph` | Обычный абзац текста. | — |
| `TokenHeading` | `heading` | Заголовок первого–шестого уровня. | `level`: уровень от `1` до `6`. |
| `TokenBlockquote` | `blockquote` | Цитата; дочерние элементы — её содержимое. | — |
| `TokenList` | `list` | Маркированный или нумерованный список. | `ordered`: `true` для нумерованного; `start`: начальный номер; `tight`: признак плотного списка. |
| `TokenListItem` | `list_item` | Один элемент списка. | — |
| `TokenCodeBlock` | `code_block` | Блок кода; содержимое находится в `Literal`. | `language`: язык из метки fenced-блока, если указан. |
| `TokenCodeSpan` | `code_span` | Фрагмент кода внутри строки; содержимое находится в `Literal`. | — |
| `TokenThematicRule` | `thematic_rule` | Горизонтальная разделительная линия (`<hr>`). | — |
| `TokenText` | `text` | Обычный текст; встроенный рендерер экранирует его для HTML. | — |
| `TokenEmphasis` | `emphasis` | Выделенный курсивом фрагмент (`<em>`). | — |
| `TokenStrong` | `strong` | Фрагмент с сильным выделением (`<strong>`). | — |
| `TokenStrikethrough` | `strikethrough` | Зачёркнутый фрагмент (`<del>`). | — |
| `TokenLink` | `link` | Ссылка; дочерние токены образуют её подпись. | `destination`: URL; `title`: необязательная подсказка. |
| `TokenImage` | `image` | Изображение; альтернативный текст находится в `Literal`. | `destination`: URL изображения; `title`: необязательная подсказка. |
| `TokenAutolink` | `autolink` | Автоматически распознанный URL или адрес электронной почты. | `destination`: URL ссылки. |
| `TokenSoftbreak` | `softbreak` | Мягкий перенос строки; по умолчанию выводит перевод строки. | — |
| `TokenHardbreak` | `hardbreak` | Принудительный перенос строки (`<br>`). | — |
| `TokenHTMLBlock` | `html_block` | HTML-блок из исходного текста. По умолчанию экранируется. | — |
| `TokenHTMLInline` | `html_inline` | Inline HTML из исходного текста. По умолчанию экранируется. | — |
| `TokenTable` | `table` | Таблица. | — |
| `TokenTableHead` | `table_head` | Заголовочная часть таблицы (`<thead>`). | — |
| `TokenTableBody` | `table_body` | Основная часть таблицы (`<tbody>`). | — |
| `TokenTableRow` | `table_row` | Строка таблицы (`<tr>`). | — |
| `TokenTableCell` | `table_cell` | Ячейка заголовка (`<th>`) или данных (`<td>`). | `header`: `true` для заголовочной ячейки; `alignment`: `left`, `center` или `right`, если задано. |
| `TokenTaskCheckbox` | `task_checkbox` | Неактивный флажок пункта task list. | `checked`: `true`, если пункт отмечен. |

## Константы атрибутов

Используйте эти константы вместо строковых имён в `Token.Attributes`:

| Константа | Строковое имя | Используется для |
| --- | --- | --- |
| `AttributeDestination` | `destination` | URL ссылки, изображения или autolink. |
| `AttributeTitle` | `title` | Необязательная подсказка ссылки или изображения. |
| `AttributeLevel` | `level` | Уровень заголовка `1`–`6`. |
| `AttributeOrdered` | `ordered` | Тип списка: `true` означает нумерованный. |
| `AttributeStart` | `start` | Начальное число нумерованного списка. |
| `AttributeLanguage` | `language` | Язык блока кода. |
| `AttributeAlignment` | `alignment` | Выравнивание ячейки таблицы. |
| `AttributeHeader` | `header` | Признак заголовочной ячейки таблицы. |
| `AttributeChecked` | `checked` | Состояние флажка task list. |
| `AttributeTight` | `tight` | Признак плотного списка. |

## Результаты и ошибки

| Тип или ошибка | Что содержит или означает |
| --- | --- |
| `TokenizationResult` | Список корневых токенов в `Tokens` и некритичные предупреждения в `Warnings`. |
| `ConversionResult` | Готовую строку HTML в `HTML` и предупреждения в `Warnings`. |
| `Diagnostic` | Предупреждение с исходной ошибкой, фазой, ID токена и диапазоном `Span`. |
| `ParseError` | Ошибку с фазой, ID правила/токена и диапазоном исходного текста; оборачивает исходную ошибку. |
| `ErrFrozen` | Попытка изменить конвертер после начала обработки. |
| `ErrInvalidUTF8` | Вход содержит некорректную UTF-8 последовательность. |
| `ErrLimitExceeded` | Превышен один из настроенных лимитов. |
| `ErrRuleProgress` | Пользовательское правило сообщило неверное продвижение по входу. |
| `ErrUnknownToken` | Для токена не найден обработчик рендеринга. |

Фазы `RulePhaseBlock` и `RulePhaseInline` задают тип пользовательского правила;
`RulePhaseRender` указывает на ошибку рендеринга в диагностике. Совпавшие
правила с большим `Priority` запускаются раньше. При одинаковом приоритете
раньше запускается зарегистрированное позже правило. Совпавшее правило должно
потребить положительное число UTF-8 байтов; правило блока должно завершаться
на границе целой строки.
