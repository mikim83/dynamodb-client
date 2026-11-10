# dynamodb-client

A read-only terminal UI to browse Amazon DynamoDB tables, using the profiles
and roles already configured in `~/.aws`.

- One static binary for Linux, macOS and Windows (amd64 and arm64). Nothing
  else to install.
- Credentials are resolved exactly like the AWS CLI: profiles from
  `~/.aws/config` and `~/.aws/credentials`, `role_arn` + `source_profile`
  (with an MFA prompt if the role needs it), SSO (`aws sso login`),
  `credential_process`, environment variables and instance roles.
- Paged scan (50 rows per page by default), with fast forward/back paging.
- Sort by any column. The current page is sorted at once; press `a` to load
  the whole table and sort it globally.
- Never writes: it only calls `ListTables`, `DescribeTable` and `Scan`.

## Install

Download the archive for your platform from the
[releases](https://github.com/mikim83/dynamodb-client/releases), extract it
and put `dynamodb-client` (or `dynamodb-client.exe`) somewhere in your `PATH`.

Or build it with Go:

```sh
go install github.com/mikim83/dynamodb-client@latest
```

On macOS, a downloaded binary may be quarantined by Gatekeeper; clear it with
`xattr -d com.apple.quarantine dynamodb-client`.

## Usage

```sh
dynamodb-client                          # pick a profile, then a table
dynamodb-client --profile prod           # or AWS_PROFILE=prod dynamodb-client
dynamodb-client --profile prod --table orders
dynamodb-client --endpoint-url http://localhost:8000 --region us-east-1   # DynamoDB Local
```

| Flag | Default | |
|---|---|---|
| `--profile` | `$AWS_PROFILE` | Profile to use. Without it, and with more than one profile, a picker is shown. |
| `--region` | profile's region | Region to use. |
| `--endpoint-url` | | Custom endpoint (DynamoDB Local, LocalStack…). |
| `--table` | | Open this table directly. |
| `--page-size` | `50` | Rows per page (1–1000). |
| `--max-items` | `10000` | Cap for "load all". |
| `--version` | | Print the version. |

## Keys

**Tables:** `enter` open · `/` filter · `r` reload · `P` switch profile · `q` quit

**Items:**

| Key | Action |
|---|---|
| `→` `n` `pgdown` | next page |
| `←` `p` `pgup` | previous page |
| `g` / `G` | first page / last page (after `a`) |
| `↑` `↓` `k` `j` | move the row cursor |
| `tab` / `shift+tab` | select the next / previous column |
| `s` | sort by the selected column: ascending → descending → off |
| `S` | clear the sort |
| `a` | load the whole table (up to `--max-items`) so sorting and paging cover all of it |
| `enter` | show the item as JSON |
| `r` | reload from the first page |
| `esc` | back |
| `?` | all keys |

### How sorting works

DynamoDB `Scan` returns items in no particular order and pages only forward,
so there is no server-side "sort by any attribute". While paging, `s`
reorders the rows of the page on screen (the status bar says
*this page*). Press `a` to read the whole table into memory: sorting then
covers every item and the pages are slices of the sorted result (*all
loaded*). Items without the sorted attribute always go last. Numbers are
compared numerically.

The item count shown is `DescribeTable`'s, which DynamoDB refreshes about
every six hours, so it is approximate.

## Permissions

The identity needs only:

```json
{
  "Effect": "Allow",
  "Action": ["dynamodb:ListTables", "dynamodb:DescribeTable", "dynamodb:Scan"],
  "Resource": "*"
}
```

Scope `DescribeTable` and `Scan` to specific table ARNs if you prefer;
`ListTables` does not support resource-level permissions.

## Development

```sh
go test ./...
go run . --profile dev
```

Releases are built by [GoReleaser](https://goreleaser.com) when a `v*` tag is
pushed (`.github/workflows/release.yml`).

## License

[GNU General Public License v3.0](LICENSE).
