# Documentation media

The root README uses `demo.gif` as its hero and the PNG files as its interface
gallery. Capture them from a true-color terminal at roughly 112 columns by
24–26 rows so wide layouts and footer controls remain visible. Crop published
assets to the terminal content; do not include the operating-system title bar,
window controls, or outer shadow.

The build-tagged documentation harness runs the real TUI renderer with
fictional data:

```sh
go run -tags tui_demo ./cmd/tuidemo -screen dashboard
go run -tags tui_demo ./cmd/tuidemo -screen browse
go run -tags tui_demo ./cmd/tuidemo -first-backup
```

Supported static screen names are `dashboard`, `backup`, `browse`, `compare`,
`restore`, `sync`, `tenants`, and `settings`. The first-backup mode intercepts
the start action and simulates progress in-process, so documentation capture
cannot write to Microsoft Graph.

Keep tenant names and content fictional, use the same terminal dimensions for
the full set, and visually inspect every regenerated asset before publishing.
