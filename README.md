
# chkbit

chkbit is a tool that ensures the safety of your files by checking if their *data integrity remains intact over time*, especially during transfers and backups. It helps detect issues like disk damage, filesystem errors, and malware interference.

![gif of chkbit](https://raw.githubusercontent.com/laktak/chkbit-py/readme/readme/chkbit-py.gif "chkbit")

- [How it works](#how-it-works)
- [Installation](#installation)
- [Usage](#usage)
- [Repair](#repair)
- [Ignore files](#ignore-files)
- [FAQ](#faq)
- [Development](#development)


## How it works

- **On your Disk**: chkbit starts by creating checksums for each folder on your main disk. It alerts you to potential problems such as damage on the disk, filesystem errors, and malware attacks that could alter your files.

- **On your Backup**: Regardless of your storage media, chkbit stores indexes in hidden files alongside your data during backups. When you run chkbit on your backup, it verifies that every byte was accurately transferred. If issues like [bitrot/data degradation](https://en.wikipedia.org/wiki/Data_degradation) occur, chkbit helps identify damaged files, alerting you to replace them with other backups.

- **For Data in the Cloud**: chkbit is useful for cloud-stored data, alerting you to any changes introduced by cloud providers like video re-encoding or image compression. It ensures your files remain unchanged in the cloud.

Remember to always maintain multiple backups for comprehensive data protection.


## Installation

- Install via [Homebrew](https://formulae.brew.sh/formula/chkbit) for macOS and Linux:
  ```
  brew install chkbit
  ```
- Download for [Linux, macOS or Windows](https://github.com/laktak/chkbit-py/releases).
- Get it with [pipx](https://pipx.pypa.io/latest/installation/)
  ```
  pipx install chkbit
  ```


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

chkbit is designed to detect "damage". To repair your files you need to think ahead:

- backup regularly
- run chkbit *before* each backup
- run chkbit *after* a backup on the backup media (readonly)
- in case of any issues, *restore* from a checked backup medium.


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
pipenv run python3 run.py
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
