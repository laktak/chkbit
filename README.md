# chkbit

chkbit is a lightweight **bitrot detection tool**.

chkbit is independent of the file system and can help you detect bitrot on you primary system, on backups and in the cloud.

## What is bitrot?

0 bits flipped | 1 bit flipped  | 2 bits flipped | 3 bits flipped
-------------- | -------------- | -------------- | --------------
![](https://upload.wikimedia.org/wikipedia/commons/thumb/2/2f/Bitrot_in_JPEG_files%2C_0_bits_flipped.jpg/180px-Bitrot_in_JPEG_files%2C_0_bits_flipped.jpg) | ![](https://upload.wikimedia.org/wikipedia/commons/thumb/1/1a/Bitrot_in_JPEG_files%2C_1_bit_flipped.jpg/180px-Bitrot_in_JPEG_files%2C_1_bit_flipped.jpg) | ![](https://upload.wikimedia.org/wikipedia/commons/thumb/3/37/Bitrot_in_JPEG_files%2C_2_bits_flipped.jpg/180px-Bitrot_in_JPEG_files%2C_2_bits_flipped.jpg) | ![](https://upload.wikimedia.org/wikipedia/commons/thumb/d/dd/Bitrot_in_JPEG_files%2C_3_bits_flipped.jpg/180px-Bitrot_in_JPEG_files%2C_3_bits_flipped.jpg) |

Data degradation (aka bitrot) is the gradual corruption of computer data due to an accumulation of non-critical failures in a data storage device. It results from the gradual decay of storage media over the course of years or longer. Causes vary by medium.

**For more information** see [Wikipedia - Data_degradation](https://en.wikipedia.org/wiki/Data_degradation).

This is the successor to [chkbit/node](https://github.com/laktak/chkbit). It will use and upgrade the index files created by the node version.

## Installation

```
pip install --user chkbit
```

Or in its own environment:

```
pipx install chkbit
```

## Usage

Run `chkbit -u PATH` to create/update the chkbit index.

chkbit will

- create a `.chkbit` index in every subdirectory of the path it was given.
- update the index with md5 hashes for every file.
- report bitrot for files that rotted since the last run (check the exit status).

Run `chkbit PATH` to verify only.

```
usage: chkbit.py [-h] [-u] [-f] [-q] [-v] PATH [PATH ...]

Checks files for bitrot. See https://github.com/laktak/chkbit-py

positional arguments:
  PATH

optional arguments:
  -h, --help     show this help message and exit
  -u, --update   update indices (without this chkbit will only verify files)
  -f, --force    force update of damaged items
  -q, --quiet    quiet, don't show progress/information
  -v, --verbose  verbose output

Status codes:
  ROT: error, bitrot detected
  EIX: error, index damaged
  old: warning, file replaced by an older version
  new: new file
  upd: file updated
  ok : check ok
  skp: skipped (see .chkbitignore)
  EXC: internal exception
```

## Repair

chkbit cannot repair bitrot, its job is simply to detect it.

You should

- backup regularly.
- run chkbit *before* each backup.
- check for bitrot on the backup media.
- in case of bitrot *restore* from a checked backup.

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

When run for the first time it records a md5 hash of the file contents as well as the file modification time.

When you run it again it first checks the modification time,

- if the time changed (because you made an edit) it records a new md5 hash.
- otherwise it will compare the current md5 to the recorded value and report an error if they do not match.

### Can I test if chkbit is working correctly?

On Linux/OS X you can try:

Create test and set the modified time:
```
$ echo foo1 > test; touch -t 201501010000 test
$ chkbit -u .
add ./test
Processed 1 file(s).
Indices were updated.
```
`add` indicates the file was added.

Now update test with a new modified:
```
$ echo foo2 > test; touch -t 201501010001 test # update test & modified
$ chkbit -u .
upd ./test
Processed 1 file(s).
Indices were updated.
```

`upd` indicates the file was updated.

Now update test with the same modified to simulate bitrot:
```
$ echo foo3 > test; touch -t 201501010001 test
$ chkbit -u .
ROT ./test
Processed 0 file(s).
chkbit detected bitrot in these files:
./test
error: detected 1 file(s) with bitrot!
```

`ROT` indicates bitrot.

