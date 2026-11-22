# Pages, dialogs, services and state

## The page

`main` receives a `*flet.Page`: one per desktop window or browser tab.

```go
page.Title = "My app"
page.ThemeMode = flet.ThemeModeDark
page.Theme = &flet.Theme{ColorSchemeSeed: "indigo"}
page.Fonts = map[string]string{"Kanit": "fonts/Kanit-Bold.ttf"}
page.Update()
```

The page settings and events come from Flet's `Page` (`Theme`, `DarkTheme`,
`ThemeMode`, `Fonts`, `LocaleConfiguration`, `OnResize`, `OnKeyboardEvent`,
`OnPlatformBrightnessChange`, `OnAppLifecycleStateChange`...).

Read-only values reported by the client:

| Field | Meaning |
|---|---|
| `Width`, `Height` | Page size (updated when `OnResize` is set) |
| `Platform` | `flet.PagePlatformMacos`, `...Windows`, `...Android`... |
| `PlatformBrightness` | Light or dark system theme |
| `Web` | Running in a browser |
| `Route` | Current route, e.g. `/products/42` |
| `ClientIP`, `ClientUserAgent` | The browser (web only) |

## Views

`page.Views` is the stack of screens; the last one is visible. Each `flet.View`
has its own `AppBar`, `NavigationBar`, `Drawer`, `FloatingActionButton`,
`Padding`, `Bgcolor`, `Scroll` and `Controls`.

```go
page.Views[0].AppBar = &flet.AppBar{Title: "Home"}
page.Add(&flet.Text{Value: "content"}) // adds to page.Views[0]
```

With several screens, use the [router](routing.md) instead of changing
`page.Views` by hand.

## Dialogs

```go
dlg := &flet.AlertDialog{
	Title:   "Delete?",
	Content: &flet.Text{Value: "This cannot be undone."},
}
dlg.Actions = []flet.Control{
	&flet.TextButton{Content: "Cancel", OnClick: func(*flet.Event) { page.PopDialog() }},
}
page.ShowDialog(dlg)
```

`ShowDialog` works for `AlertDialog`, `BottomSheet`, `SnackBar`, `Banner`,
`DatePicker`, `TimePicker` and the Cupertino dialogs. The dialog leaves the page
when the user closes it; its `OnDismiss` runs then.

`page.Overlay` holds controls drawn above the page.

## Services

Services are controls without a screen: `Clipboard`, `FilePicker`,
`SharedPreferences`, `UrlLauncher`, `HapticFeedback`, `Wakelock`... Add them to
`page.Services` before calling their methods:

```go
clipboard := &flet.Clipboard{}
page.Services = append(page.Services, clipboard)
page.Update()

text, err := clipboard.Get()
```

## Window (desktop)

`page.Window` controls the native window:

```go
page.Window.Width = 800
page.Window.Height = 600
page.Update()
page.Window.Center()
```

## Updating from other goroutines

Handlers of a page run one at a time. Code in other goroutines (timers, chat
messages, background jobs) must go through `page.Run`, which runs a function on the
same queue:

```go
go func() {
	for msg := range messages {
		page.Run(func() {
			chat.Controls = append(chat.Controls, &flet.Text{Value: msg})
			page.Update()
		})
	}
}()
```

## Per-session values

`flet.NewSessionKey` stores a typed value per page (per browser tab), shared by
every route:

```go
var cart = flet.NewSessionKey(func() *Cart { return &Cart{} })

func Checkout(ctx *flet.RouteContext) flet.Control {
	c := cart.Get(ctx.Page) // created on first use
	...
}
```

`key.Set(page, value)` replaces the value.

## Connection events

| Field | Runs when |
|---|---|
| `page.OnConnect` | The client reconnects to this page |
| `page.OnDisconnect` | The connection drops |
| `page.OnClose` | A disconnected web session expires: stop its goroutines here |
