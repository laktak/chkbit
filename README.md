
# chkbit

chkbit is a lightweight tool to check the data integrity of your files. It allows you to verify *that the data has not changed* since you put it there and that it is still the same when you move it somewhere else.

cross-platform support for [Linux, macOS and Windows](https://github.com/laktak/chkbit-py/releases)!

- [Use it](#use-it)
  - [On your Disk](#on-your-disk)
  - [On your Backup](#on-your-backup)
  - [For Data in the Cloud](#for-data-in-the-cloud)
- [Installation](#installation)
- [Usage](#usage)
- [Repair](#repair)
- [Ignore files](#ignore-files)
- [FAQ](#faq)
  - [Should I run `chkbit` on my whole drive?](#should-i-run-chkbit-on-my-whole-drive)
  - [Why is chkbit placing the index in `.chkbit` files (vs a database)?](#why-is-chkbit-placing-the-index-in-chkbit-files-vs-a-database)
  - [How does chkbit work?](#how-does-chkbit-work)
  - [I wish to use a stronger hash algorithm](#i-wish-to-use-a-stronger-hash-algorithm)
  - [How can I delete the index files?](#how-can-i-delete-the-index-files)
  - [Can I test if chkbit is working correctly?](#can-i-test-if-chkbit-is-working-correctly)
- [Development](#development)

## Use it

### On your Disk

chkbit starts with your primary disk. It creates checksums for each folder that will follow your data onto your backups.

Here it alerts you to
- damage on the disk
- damage caused by filesystem errors
- damage caused by malware (when it encrypts your files)

The built in checksums from your filesystems only cover some of these cases.

### On your Backup

No matter what storage media or filesystem you use, chkbit stores its indexes in hidden files that are backed up together with your data.

When you run chkbit on your backup media you can verify that every byte was correctly transferred.

If your backup media fails or experiences [bitrot/data degradation](https://en.wikipedia.org/wiki/Data_degradation), chkbit allows you to discover what files were damaged and need to be replaced by other backups. You should always keep multiple backups :)

### For Data in the Cloud

Some cloud providers re-encode your videos or compress your images to save space. chkbit will alert you of any changes.

## Installation

Download: You can download a release directly from [github releases](https://github.com/laktak/chkbit-py/releases).

If you OS/platform is not yet supported you can also use either [pipx](https://pipx.pypa.io/latest/installation/) or pip:

- `pipx install chkbit`
- `pip install --user chkbit`

## Usage

Run `chkbit -u PATH` to create/update the chkbit index.

chkbit will

- create a `.chkbit` index in every subdirectory of the path it was given.
- update the index with blake3 (see --algo) hashes for every file.
- report damage for files that failed the integrity check since the last run (check the exit status).

Run `chkbit PATH` to verify only.

```
usage: chkbit [-h] [-u] [--show-ignored-only] [--algo ALGO] [-f] [-s] [-l FILE] [--log-verbose] [--index-name NAME] [--ignore-name NAME] [-w N] [--plain] [-q] [-v] [PATH ...]

Checks the data integrity of your files. See https://github.com/laktak/chkbit-py

positional arguments:
  PATH                  directories to check

options:
  -h, --help            show this help message and exit
  -u, --update          update indices (without this chkbit will verify files in readonly mode)
  --show-ignored-only   only show ignored files
  --algo ALGO           hash algorithm: md5, sha512, blake3 (default: blake3)
  -f, --force           force update of damaged items
  -s, --skip-symlinks   do not follow symlinks
  -l FILE, --log-file FILE
                        write to a logfile if specified
  --log-verbose         verbose logging
  --index-name NAME     filename where chkbit stores its hashes, needs to start with '.' (default: .chkbit)
  --ignore-name NAME    filename that chkbit reads its ignore list from, needs to start with '.' (default: .chkbitignore)
  -w N, --workers N     number of workers to use (default: 5)
  --plain               show plain status instead of being fancy
  -q, --quiet           quiet, don't show progress/information
  -v, --verbose         verbose output

.chkbitignore rules:
  each line should contain exactly one name
  you may use Unix shell-style wildcards (see README)
  lines starting with `#` are skipped
  lines starting with `/` are only applied to the current directory

Status codes:
  DMG: error, data damage detected
  EIX: error, index damaged
  old: warning, file replaced by an older version
  new: new file
  upd: file updated
  ok : check ok
  ign: ignored (see .chkbitignore)
  EXC: internal exception
```

chkbit is set to use only 5 workers by default so it will not slow your system to a crawl. You can specify a higher number to make it a lot faster if the IO throughput can also keep up.

## Repair

chkbit cannot repair damage, its job is simply to detect it.

You should

- backup regularly.
- run chkbit *before* each backup.
- check for damage on the backup media.
- in case of damage *restore* from a checked backup.

## Ignore files

Add a `.chkbitignore` file containing the names of the files/directories you wish to ignore

- each line should contain exactly one name
- you may use [Unix shell-style wildcards](https://docs.python.org/3/library/fnmatch.html)
  - `*` matches everything
  - `?`  matches any single character
  - `[seq]` matches any character in seq
  - `[!seq]` matches any character not in seq
- lines starting with `#` are skipped
- lines starting with `/` are only applied to the current directory
- you can use `path/sub/name` to ignore a file/directory in a sub path
- hidden files (starting with a `.`) are ignored by default

## FAQ

### Should I run `chkbit` on my whole drive?

You would typically run it only on *content* that you keep for a long time (e.g. your pictures, music, videos).

### Why is chkbit placing the index in `.chkbit` files (vs a database)?

The advantage of the .chkbit files is that

- when you move a directory the index moves with it
- when you make a backup the index is also backed up

The disadvantage is obviously that you get hidden `.chkbit` files in your content folders.

### How does chkbit work?

chkbit operates on files.

When run for the first time it records a hash of the file contents as well as the file modification time.

When you run it again it first checks the modification time,

- if the time changed (because you made an edit) it records a new hash.
- otherwise it will compare the current hash to the recorded value and report an error if they do not match.

### I wish to use a different hash algorithm

chkbit now uses blake3 by default. You can also specify `--algo sha512` or `--algo md5`.

Note that existing index files will use the hash that they were created with. If you wish to update all hashes you need to delete your existing indexes first. A conversion mode may be added later (PR welcome).

### How can I delete the index files?

List them with

```
find . -name .chkbit
```

and add `-delete` to delete.

### Can I test if chkbit is working correctly?

On Linux/macOS you can try:

Create test and set the modified time:
```
$ echo foo1 > test; touch -t 201501010000 test
$ chkbit -u .
new ./test

Processed 1 file.
- 0:00:00 elapsed
- 192.31 files/second
- 0.00 MB/second
- 1 directory was updated
- 1 file hash was added
- 0 file hashes were updated
```

`new` indicates a new file was added.

Now update test with a new modified:
```
$ echo foo2 > test; touch -t 201501010001 test # update test & modified
$ chkbit -u .
upd ./test

Processed 1 file.
- 0:00:00 elapsed
- 191.61 files/second
- 0.00 MB/second
- 1 directory was updated
- 0 file hashes were added
- 1 file hash was updated
```

`upd` indicates the file was updated.

Now update test with the same modified to simulate damage:
```
$ echo foo3 > test; touch -t 201501010001 test
$ chkbit -u .
DMG ./test

Processed 1 file.
- 0:00:00 elapsed
- 173.93 files/second
- 0.00 MB/second
chkbit detected damage in these files:
./test
error: detected 1 file with damage!
```

`DMG` indicates damage.

## Development

With pipenv (install with `pipx install pipenv`):

```
# setup
pipenv install

# run chkbit
pipenv run python3 -m chkbit_cli.main
```

To build a source distribution package from pyproject.toml
```
pipx run build
```

You can then install your own package with
```
pipx install dist/chkbit-*.tar.gz
```

The binaries are created using pyinstaller via Github actions.
