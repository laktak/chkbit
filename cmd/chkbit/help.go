package main

var headerHelp = `Ensures the safety of your files by verifying that their data integrity remains intact over time, especially during transfers and backups.

 For help tips run "chkbit -H" or go to
 https://github.com/laktak/chkbit
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
  DMG: error, data damage detected
  EIX: error, index damaged
  old: warning, file replaced by an older version
  new: new file
  upd: file updated
  ok : check ok
  del: file/directory removed
  ign: ignored (see .chkbitignore)
  EXC: exception/panic
`
