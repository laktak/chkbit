# chkbit

chkbit is a lightweight tool to check the data integrity of your files. It allows you to verify *that the data has not changed* since you put it there and that it is still the same when you move it somewhere else.

### On your Disk

chkbit starts with your primary disk. It creates checksums for each folder that will follow your data onto your backups.

Even though your filesystems should have built in checksums, it is usually not trivial to take them onto another media.

### On your backup

No matter what storage media or filesystem you use, chkbit stores its indexes in hidden files that are backed up together with your data.

When you run chkbit-verify on your backup media you can make sure that every byte was correctly transferred.

If your backup media fails or experiences [bitrot/data degradation](https://en.wikipedia.org/wiki/Data_degradation), chkbit allows you to discover what files were damaged and need to be replaced by other backups.

### Data in the Cloud

Some cloud providers re-encode your videos or compress your images to save space. chkbit will alert you of any changes.

## Installation

The easiest way to install python CLI tools is with [pipx](https://pipx.pypa.io/latest/installation/).

```
pipx install chkbit
```

You can also use pip:

```
pip install --user chkbit
```

**NOTE** version 3 now uses the blake3 hash algorithm by default as it is not only better but also faster than md5.

## Usage

Run `chkbit -u PATH` to create/update the chkbit index.

chkbit will

- create a `.chkbit` index in every subdirectory of the path it was given.
- update the index with blake3 (see --algo) hashes for every file.
- report damage for files that failed the integrity check since the last run (check the exit status).

Run `chkbit PATH` to verify only.

```
usage: chkbit [-h] [-u] [--algo ALGO] [-f] [-s] [--index-name NAME] [--ignore-name NAME] [-w N] [--plain] [-q] [-v] [PATH ...]

Checks the data integrity of your files. See https://github.com/laktak/chkbit-py

positional arguments:
  PATH                 directories to check

options:
  -h, --help           show this help message and exit
  -u, --update         update indices (without this chkbit will verify files in readonly mode)
  --algo ALGO          hash algorithm: md5, sha512, blake3 (default: blake3)
  -f, --force          force update of damaged items
  -s, --skip-symlinks  do not follow symlinks
  --index-name NAME    filename where chkbit stores its hashes (default: .chkbit)
  --ignore-name NAME   filename that chkbit reads its ignore list from (default: .chkbitignore)
  -w N, --workers N    number of workers to use (default: 5)
  --plain              show plain status instead of being fancy
  -q, --quiet          quiet, don't show progress/information
  -v, --verbose        verbose output

Status codes:
  DMG: error, data damage detected
  EIX: error, index damaged
  old: warning, file replaced by an older version
  new: new file
  upd: file updated
  ok : check ok
  skp: skipped (see .chkbitignore)
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
- lines starting with `#` are skipped
- you may use [Unix shell-style wildcards](https://docs.python.org/3.8/library/fnmatch.html)

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

### I wish to use a stronger hash algorithm

chkbit now uses blake3 by default. You can also specify it with `--algo sha512` or `--algo md5`.

Note that existing index files will use the hash that they were created with. If you wish to update all hashes you need to delete your existing indexes first.

### How can I delete the index files?

List them with

```
find . -name .chkbit
```

and add `-delete` to delete.

### Can I test if chkbit is working correctly?

On Linux/OS X you can try:

Create test and set the modified time:
```
$ echo foo1 > test; touch -t 201501010000 test
$ chkbit -u .
new ./test

Processed 1 file.
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
- 173.93 files/second
- 0.00 MB/second
chkbit detected damage in these files:
./test
error: detected 1 file with damage!
```

`DMG` indicates damage.

