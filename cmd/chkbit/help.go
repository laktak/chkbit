package main

var headerHelp = `Alerts you of data corruption in your files, especially during transfers, backups and after recovery.

 For help tips run "chkbit -H" or go to
 https://github.com/laktak/chkbit
 Author: Christian Zangl <laktak@cdak.net>
`

var helpTips = `
.chkbitignore rules:
- each line should contain exactly one name
- you may use Unix shell-style wildcards
  - *       matches everything except /
  - ?       matches any single character except /
  - [seq]   matches any character/range in seq
  - [^seq]  matches any character/range not in seq
  - \\      escape to match the following character
- lines starting with '#' are skipped
- lines starting with '/' are only applied to the current directory

Status codes:
  PNC: exception/panic, unable to continue
  DMG: error, data damage detected
  ERX: error, index damaged
  old: warning, file replaced by an older version
  upd: file updated
  new: new file
  ok : checked and ok (verbose)
  del: file/directory removed (-m)
  ign: ignored (see .chkbitignore)
  msg: message

Configuration file (json):
- location <config-file>
- key names are the option names with '-' replaced by '_'
- for example --include-dot is written as:
  { "include_dot": true }

Performance:
- chkbit uses 5 workers by default. To speed it up tune it with the --workers flag.
- Note: slow/spinning disks work best with just 1 worker!

Deduplication requires a supported OS:
- Linux with a CoW filesystem like btrfs (full support)
- macOS with a APFS (detect supported)
- for details see https://laktak.github.io/chkbit/dedup/
`
