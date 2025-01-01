
# chkbit

chkbit alerts you of data corruption of your files, especially during transfers, backups and after recovery. It helps detect issues like disk damage, filesystem errors, and malware interference.

Some filesystems (like Btrfs and ZFS, but not APFS or NTFS) already protect your files with checksums. However when you move files between locations, separate checks have the advantage of confirming that the data was not modified during transit. So you know the photo on your disk is the same as the copy in your cloud backup. This also protects you from overwriting good data with bad copies.


![gif of chkbit](https://raw.githubusercontent.com/wiki/laktak/chkbit/readme/chkbit.gif "chkbit")

- [How it works](#how-it-works)
- [Usage](#usage)
- [Atom vs Split Mode](#atom-vs-split-mode)
- [Repair](#repair)
- [Ignore files](#ignore-files)
- [Installation](#installation)
- [chkbit as a Go module](#chkbit-as-a-go-module)
- [FAQ](#faq)


## How it works

- **On your Disk**: chkbit starts by creating checksums for each or selected files on your disk. It alerts you to potential problems such as damage on the disk, filesystem errors, and malware attacks that could alter your files.

- **On your Backup**: Regardless of your storage media, chkbit stores index files alongside your data during backups. When you run chkbit on your backup, it verifies that every byte was accurately transferred. If issues like [bitrot/data degradation](https://en.wikipedia.org/wiki/Data_degradation) occur, chkbit helps identify damaged files, alerting you to replace them with other backups.

- **For Data in the Cloud**: chkbit is useful for cloud-stored data, alerting you to any changes introduced by cloud providers like video re-encoding or image compression. It ensures that you notice any changes to your files in the cloud.

Remember to always maintain multiple backups for comprehensive data protection.


## Usage

First initialize the directory tree you wish to check.

Here you can decide to run chkbit in

- `split` mode, where it stores an index in each directory, or
- `atom` mode, where the index is stored in a single file.

See pro and cons below.

For example go to the `documents` folder, then run

```
chkbit init atom .
```

This will create the index in the current directory that also serves all subfolders. If you know git, this is the same concept.


To add files to your index run update:

```
chkbit update .
```

chkbit will search the current and all subfolders, create hashes and add them to the store. It will also check existing hashes (skip with `-s`).

To only verify your files run

```
chkbit check .
```

This is mainly used on your backup to verify that your files are intact (use `--workers=1` on spinning disks).

For more info run

```
chkbit --help   # shows flags
chkbit tips     # status codes and ignore syntax
chkbit init -h  # shows flags and help for this command
```


## Atom vs Split Mode

In `atom` mode, chkbit uses a single `.chkbit-db` file to store all hashes (referred to as index):

- pro: it does not clutter your system with hidden index files
- con: you need to make sure to include `.chkbit-db` with your backup
- con: when you move folders, the hashes do not move with them
- con: if the index is damaged it affects the all hashes (manual recovery possible)

In `split` mode, chkbit creates a `.chkbit` file for the hashes in every folder (unless ignored):

- pro: when you make a backup, even for partial backups, the correct hashes are also backed up
- pro: if one index is damaged the others are still fine
- pro when you move a directory the index moves with it
- con: while hidden, the `.chkbit` files are present in every directory

In both modes the hashes are save in a json file. This is a future proof format that you can easily extract your hashes from. Since the hashes are standard algorithms you can check your files even if you can't get a copy of chkbit on some system in the future.


## Repair

chkbit is designed to detect "damage". To repair your files you need to think ahead:

- backup regularly
- run chkbit *before* each backup
- run chkbit *after* a backup on the backup media (readonly)
- in case of any issues, *restore* from a checked backup medium.


## Ignore files

Add a `.chkbitignore` file containing the names of the files/directories you wish to ignore

- each line should contain exactly one name
- you may use Unix shell-style wildcards
  - `*` matches everything except `/`
  - `?`  matches any single character except `/`
  - `[seq]` matches any character/range in seq
  - `[^seq]` matches any character/range not in seq
  - `\\` escape to match the following character
- lines starting with `#` are skipped
- lines starting with `/` are only applied to the current directory
- you can use `path/sub/name` to ignore a file/directory in a sub path
- hidden files (starting with a `.`) are ignored by default unless you use the `-d` option



## Installation

### Binary releases

You can download the official chkbit binaries from the releases page and place it in your `PATH`.

- https://github.com/laktak/chkbit/releases

Prereleased versions can be found directly on the [GitHub Action](https://github.com/laktak/chkbit/actions). Click on the latest `ci` action and look for `prerelease-artifacts` at the bottom.

### Homebrew (macOS and Linux)

For macOS and Linux it can also be installed via [Homebrew](https://formulae.brew.sh/formula/chkbit):

```shell
brew install chkbit
```

### Build from Source

Building from the source requires Go.

- Either install it directly

```shell
go install github.com/laktak/chkbit/v5/cmd/chkbit@latest
```

- or clone and build

```shell
git clone https://github.com/laktak/chkbit
chkbit/scripts/build
# binary:
ls -l chkbit/chkbit
```


## chkbit as a Go module

chkbit is can also be used in other Go programs.

```
go get github.com/laktak/chkbit/v5
```

For more information see the documentation on [pkg.go.dev](https://pkg.go.dev/github.com/laktak/chkbit/v5).


## FAQ

### Should I run `chkbit` on my whole drive?

You would typically run it only on *content* that you keep for a long time (e.g. your pictures, music, videos).

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

On Linux/macOS you can try the following.

Create a directory and initialize it:

```
$ mkdir /tmp/test
$ cd /tmp/test
$ chkbit init split .
```

Create test and set the modified time:
```
$ echo foo1 > test; touch -t 201501010000 test
$ chkbit update .
new test

Processed 1 file
- 0s elapsed
- 1623.70 files/second
- 0.01 MB/second
- 1 directory was updated
- 1 file hash was added
- 0 file hashes were updated
```

`new` indicates a new file was added.

Now update test with a new modified:
```
$ echo foo2 > test; touch -t 201501010001 test # update test & modified
$ chkbit update .
upd test

Processed 1 file
- 0s elapsed
- 1487.17 files/second
- 0.01 MB/second
- 1 directory was updated
- 0 file hashes were added
- 1 file hash was updated
```

`upd` indicates the file was updated.

Now update test with the same modified to simulate damage:
```
$ echo foo3 > test; touch -t 201501010001 test
$ chkbit update .
DMG test

Processed 1 file
- 0s elapsed
- 0.00 files/second
- 0.01 MB/second
chkbit detected damage in these files:
test
error: detected 1 file with damage!
```

`DMG` indicates damage.

