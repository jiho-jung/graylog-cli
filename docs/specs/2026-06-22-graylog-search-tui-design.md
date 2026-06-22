# graylog-cli Search TUI Design

## Summary

Improve `graylog-cli search --tui` from a simple text list into a keyboard-driven log browser. The TUI should keep the CLI search semantics already implemented, but make result inspection faster with a split screen, row selection, arrow-key movement, and Enter-to-open pretty detail.

The approved defaults are:

- Main layout: split screen with list 80% and summary/detail panel 20%.
- Movement: hybrid page behavior. Arrow keys move inside the current page, and pressing past the first or last item loads the previous or next page.
- Detail: Enter opens a pretty-only detail view.
- Summary panel: compact key summary for the selected log.
- Refresh: manual `r` in normal TUI, automatic refresh only for `--follow --tui`.

## UX Design

The normal TUI screen has three regions:

- Header: query, page, offset, total count, effective range when available, and key help.
- Result list: compact one-line log rows. The selected row is highlighted.
- Summary panel: timestamp, level, source, message, and a small set of selected fields for the highlighted log.

Key behavior:

- `up/down`: move the selected row within the current page.
- `up` on the first row: load the previous page when available.
- `down` on the last row: load the next page when available.
- `PgUp/PgDn` and `b/n`: explicit previous/next page.
- `Enter`: open the selected log in a pretty detail screen.
- `Esc` or `Enter` in detail: return to the list screen.
- `up/down` in detail: scroll detail content.
- `r`: refresh the current page.
- `q`, `Esc` on the list screen, or `Ctrl-C`: quit.

The detail screen uses the existing pretty renderer shape: timestamp, level, source, message, then fields as key-value lines. It does not include a JSON tab in this version.

## Implementation Notes

Keep the initial implementation in `cmd/search_tui.go`. The current file is small enough that a package split would add more structure than the feature needs.

Update the TUI model to store raw `*client.Message` values in addition to rendered compact lines. This is required because the list, summary panel, and pretty detail view all need different projections of the same selected log.

Track these model states explicitly:

- Current page offset, page limit, total, and effective range.
- Selected row index.
- Screen mode: list or detail.
- Detail scroll offset.
- Loading and error state.
- Last refresh time.

Normal `--tui` should only refresh on `r`. `--follow --tui` should refresh the first page on the configured `--refresh` interval. Follow mode must avoid surprising navigation changes: automatic refresh is only active while browsing the first page. If the user moves to another page, refresh pauses until they return to page 1 or press `r`.

Errors should remain inside the TUI instead of exiting. The user can press `r` to retry or `q` to quit.

## Test Plan

Add unit tests for pure state transitions where practical:

- Arrow movement inside a page.
- Down on the last row requests the next page.
- Up on the first row requests the previous page when offset is not zero.
- Enter toggles list to detail mode when a row is selected.
- Esc or Enter returns from detail to list.
- Refresh keeps the current query and offset in normal mode.

Keep network behavior out of unit tests. Use fake fetch results for model tests where possible.

Run:

- `go test ./...`
- `go build -o graylog-cli`

Perform a real smoke test against Graylog:

- `./graylog-cli search '*' --since 15m --limit 20 --tui`
- Verify list/summary split, arrow movement, Enter pretty detail, page boundary loading, `r` refresh, and `q` quit.
- `./graylog-cli search '*' --since 15m --limit 20 --follow --refresh 10s --tui`
- Verify automatic refresh on page 1 and no automatic page jump while browsing older pages.

## Assumptions

- TUI remains optional behind `--tui`.
- The default CLI output behavior is unchanged.
- Pretty detail is sufficient for this iteration; JSON detail can be added later if needed.
- The TUI may continue using Bubble Tea directly without adding Bubbles components.
