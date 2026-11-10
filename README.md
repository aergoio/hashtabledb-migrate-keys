# hashtabledb-migrate-keys

One-time offline migration for aergo chain databases created with the old
hashtabledb layout, where consensus keys lived in external `data.db-vk-*`
files. The tool copies the latest value of every external key into the main
append-only file, makes it durable, verifies it, and deletes the vk files.

## Build

Requires Go 1.23 or newer.

```sh
cd hashtabledb-migrate-keys
go build -o hashtabledb-migrate-keys .
```

## Usage

Stop `aergosvr` first. The database lock prevents migration while it is
running.

```sh
# aergo data directory (uses <datadir>/chain/data.db)
./hashtabledb-migrate-keys -datadir /path/to/aergo/data

# or point directly at the chain database
./hashtabledb-migrate-keys -db /path/to/aergo/data/chain/data.db
```

Inspect first:

```sh
./hashtabledb-migrate-keys -datadir /path/to/aergo/data --dry-run
```

Then run the real migration:

```sh
./hashtabledb-migrate-keys -datadir /path/to/aergo/data
```

By default the vk files are copied to
`<db>.vk-backup-<timestamp>` before being deleted. Keep that backup:
after migration, older aergosvr binaries that still register these keys as
mutable will read empty values and must not be used unless the vk files are
restored first.

Options:

```text
-db string        path to data.db
-datadir string   aergo data directory
-backup           copy vk files before deleting them (default true)
-dry-run          list external keys without changing anything
-no-verify        skip reopen-and-verify after migration
```

Run the tool on every node before starting the new binary.
