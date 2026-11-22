# Web apps

## Serve

```go
err := flet.Serve(":8550", main)
```

Open <http://localhost:8550>. Each browser tab is a session with its own page. On
the first run, the Flet web client (`flet-web.tar.gz` from the Flet release) is
downloaded and cached.

## Options

```go
flet.Serve(":8550", main, flet.WebOptions{
	BasePath:      "/app/",
	AssetsDir:     "assets",
	UploadDir:     "uploads",
	MaxUploadSize: 10 << 20,
	SecretKey:     os.Getenv("UPLOAD_KEY"),
})
```

| Option | Purpose |
|---|---|
| `BasePath` | Serve the app under a path, e.g. `/app/` |
| `AssetsDir` | Folder with your images, fonts and icons |
| `UploadDir` | Enables uploads; files are saved here |
| `MaxUploadSize` | Maximum size of each uploaded file, in bytes |
| `SecretKey` | Signs upload URLs (random when empty) |

## Mount in your own server

`flet.Handler` returns an `http.Handler`:

```go
h, err := flet.Handler(main, flet.WebOptions{BasePath: "/app/"})
if err != nil {
	log.Fatal(err)
}
mux := http.NewServeMux()
mux.Handle("/app/", h)
mux.HandleFunc("/api/health", health)
log.Fatal(http.ListenAndServe(":8080", mux))
```

Mount it at the same path as `BasePath`. `/app` redirects to `/app/`.

## Assets

Files in `AssetsDir` are served next to the app:

```go
&flet.Image{Src: "logo.png"} // assets/logo.png
```

They take precedence over the web client's files, so you can replace
`favicon.png`, `icons/`, `manifest.json` or even `index.html`.

## Uploads

With `UploadDir` set, the browser can send picked files to the server:

```go
files, err := picker.PickFiles(flet.PickFilesOptions{AllowMultiple: true})
if err != nil {
	return
}
var uploads []*flet.FilePickerUploadFile
for _, f := range files {
	url, err := page.GetUploadURL(f.Name, time.Minute)
	if err != nil {
		return
	}
	uploads = append(uploads, &flet.FilePickerUploadFile{ID: f.ID, Name: f.Name, UploadURL: url})
}
picker.Upload(uploads)
```

Progress arrives in `picker.OnUpload`. Each URL is signed (HMAC-SHA256), works only
for that file name and expires. Files cannot leave `UploadDir`, even through
symbolic links, and only appear once complete.

Set `SecretKey` (or `FLET_SECRET_KEY`) so URLs keep working across restarts and
when running several servers.

## Sessions

- A reloaded tab gets its page back, at the address it was reloaded with.
- A disconnected session is kept for one hour (`FLET_SESSION_TIMEOUT`, in
  seconds), then dropped; `page.OnClose` runs.
- Sessions live in memory. With several servers behind a load balancer, use
  sticky sessions.

## Security

- The WebSocket only accepts pages from the same host.
- Upload URLs are signed and expire.
- Paths are cleaned: requests cannot read files outside the client and assets
  folders.

## Environment variables

| Variable | Purpose |
|---|---|
| `FLET_WEB_PATH` | Use an extracted web client folder (no download) |
| `FLET_WEB_URL` | Download the web client from another URL |
| `FLET_ASSETS_DIR` | Same as `AssetsDir` |
| `FLET_SESSION_TIMEOUT` | Seconds a disconnected session is kept (default 3600) |
| `FLET_MAX_UPLOAD_SIZE` | Same as `MaxUploadSize` |
| `FLET_SECRET_KEY` | Same as `SecretKey` |
| `FLETGO_DEBUG=1` | Log the messages exchanged with each tab |

## Coming next

A production build (`flet-go build web`) that embeds the web client in your binary,
compresses it, and sets the app name and icons. See [PLAN.md](../PLAN.md).
