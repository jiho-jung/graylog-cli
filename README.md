
go install github.com/jeehoon/graylog-cli@latest

## Search

```sh
graylog-cli search 'level:3 AND service:api' --since 30m --limit 20
graylog-cli search 'service:api' --since 1h --output pretty
graylog-cli search 'service:api' --since 1h --output ndjson
graylog-cli search 'service:api' --fields request_id,spc_service,spc_tier
```

`search` uses compact one-line log output by default. Use `--output pretty`
for multi-line logs, `--output json` for a JSON array, or `--output ndjson`
for one JSON object per line.
Use `--fields` to choose extra fields shown in compact and pretty output.

```sh
graylog-cli search 'service:api' --page
graylog-cli search 'service:api' --follow --refresh 10s
graylog-cli search 'service:api' --tui
```

Analysis commands:

```sh
graylog-cli search histogram 'service:api' --since 1h
graylog-cli search top source 'service:api' --since 1h --limit 10
```
