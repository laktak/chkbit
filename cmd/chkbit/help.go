package main

var headerHelp = `Checks the data integrity of your files. 
 For help tips run "chkbit -H" or go to
 https://github.com/laktak/chkbit
`

var helpTips = `
.chkbitignore rules:
  each line should contain exactly one name
  you may use Unix shell-style wildcards (see README)
  lines starting with '#' are skipped
  lines starting with '/' are only applied to the current directory

Status codes:
  DMG: error, data damage detected
  EIX: error, index damaged
  old: warning, file replaced by an older version
  new: new file
  upd: file updated
  ok : check ok
  ign: ignored (see .chkbitignore)
  EXC: exception/panic
`
