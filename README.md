# TablePlus Connections

Export database credentials from 1Password into an encrypted `.tableplusconnection` file that TablePlus can open directly.

This tool:

- Connects to your 1Password account via the desktop app integration
- Finds all items in the **Database** category
- Lets you interactively pick which ones to export (or export all)
- Writes an encrypted `.tableplusconnection` file compatible with TablePlus
- Optionally opens the export in TablePlus on macOS

---

## Status

> ⚠️ Experimental, not affiliated with TablePlus or 1Password.  
> The `.tableplusconnection` format is not documented and may change in future TablePlus releases.

---

## Requirements

- Go 1.24+ (to build/run the tool  
- A 1Password subscription
- 1Password desktop app with **Desktop App Integration** enabled (1Password beta feature)
- At least one 1Password **Database** item with the expected fields
- TablePlus installed (if you want to open the export directly)

The `-open` flag currently only works on **macOS**, because it shells out to:

```bash
open -a TablePlus <file>
````

on Darwin systems.

---

## Installation

### From source

```bash
git clone https://github.com/recoded-dev/tableplus-connections.git
cd tableplus-connections

# run directly
go run .

# or build a binary (-ldflags are optional)
go build -ldflags "-X main.version=xxx" -o tableplus-connections .
```


---

## 1Password setup

This tool looks for 1Password items with:

* **Category**: `Database` (1Password’s built-in database item type)
* **Fields** (by field ID, not just label): ([GitHub][1])

    * `hostname`
    * `port`
    * `username`
    * `password`

If any of these fields are missing for an item, that item is skipped.

### Password as a command

If the `password` field’s **title** is `"Password command"` (case-insensitive), it will be treated as a shell command instead of a literal password when imported into TablePlus.

---

## Usage

Basic usage:

```bash
tableplus-connections [flags] <1password-account-name>
```

The `<1password-account-name>` is the account identifier passed to the 1Password Go SDK’s desktop app integration.

### Flags

All flags are optional:

* `-all`, `-a`
  Export **all** discovered database items without showing the interactive UI.

* `-group-by-vault`
  Group exported connections by 1Password vault. The output file will contain a list of groups, each with its own subset of connections.

* `-output`, `-o` (default: `export`)
  Base name of the output file (without extension).
  The tool always writes `<output>.tableplusconnection`.

* `-password`, `-p` (default: `password`)
  Password used to encrypt the `.tableplusconnection` file with [RNCryptor](https://github.com/RNCryptor/RNCryptor-go). ([GitHub][1])
  You almost certainly want to override this and pick your own strong password.

* `-open`
  After writing the file, open it with the **TablePlus** app (macOS only).

You can also run:

```bash
tableplus-connections -h
```

to see the built-in flag descriptions.

---

## Examples

### Interactive export

Let the tool ask you which connections to export:

```bash
tableplus-connections my-1password-account
```

This will:

1. Connect to 1Password via the desktop app.
2. Collect all `Database` items.
3. Show an interactive terminal UI grouped by vault. ([GitHub][1])
4. Write `export.tableplusconnection` encrypted with the default password (`password`).

### Export all connections, custom filename and password

```bash
tableplus-connections \
  -all \
  -output team-connections \
  -password "something-long-and-random" \
  my-1password-account
```

* Output: `team-connections.tableplusconnection`
* Encrypted with your custom password.

### Group by vault and open in TablePlus (macOS)

```bash
tableplus-connections \
  -group-by-vault \
  -output tableplus-export \
  -password "another-strong-password" \
  -open \
  my-1password-account
```

This will:

1. Export selected connections grouped by vault.
2. Encrypt them.
3. Immediately open `tableplus-export.tableplusconnection` in TablePlus on macOS. ([GitHub][1])

---

## Importing into TablePlus

Once you have the `.tableplusconnection` file:

* On macOS: double-click the file, or
* Use **File → Import** / the relevant import entry in TablePlus, depending on your version.

TablePlus will prompt you for the encryption password you configured via `-password`.

> Note: The tool currently hardcodes `Driver: "PostgreSQL"` and some other connection defaults.

---

## How it works (high level)

* Uses the official **1Password Go SDK** with **Desktop App Integration** to:

    * Authenticate via the 1Password desktop app
    * List vaults
    * Fetch all items in the `Database` category
* Maps those items into a simplified internal representation
* Optionally shows a terminal UI to pick which connections to export.
* Converts the selection into a structure that matches TablePlus’ JSON export format
* Serializes to JSON, encrypts it with **RNCryptor**, then writes `<output>.tableplusconnection`.

---

## Limitations / TODO

* Only `Database` category items are supported.
* Driver is currently hard-coded to PostgreSQL.
* `-open` is only implemented on macOS.
* The `.tableplusconnection` format is reverse-engineered from TablePlus exports and may break if the app changes its format in future versions.

Contributions, bug reports, and ideas for additional drivers/fields are welcome!
