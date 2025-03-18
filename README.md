
# chkbit

chkbit alerts you to data corruption in your files, especially during transfers, backups, and after recovery. It helps detect issues like disk damage, filesystem errors, and malware interference.

The latest version can also detect duplicate files and run deduplication on supported systems.

![gif of chkbit](https://raw.githubusercontent.com/wiki/laktak/chkbit/readme/chkbit.gif "chkbit")

[Chkbit Introduction](https://laktak.github.io/chkbit/)
- [Install](https://laktak.github.io/chkbit/get/)
- [How does it work?](https://laktak.github.io/chkbit/how/)
- [File Deduplication](https://laktak.github.io/chkbit/dedup/)
- [Usage](https://laktak.github.io/chkbit/usage/)
- [FAQ](https://laktak.github.io/chkbit/faq/)

## Latest Releases

### version 6.3

- new file deduplication command!

### version 6

- chkbit adds a new `atom` mode to store all indices in a single file
- there is a new `fuse` command to merge split indexes into an atom
- If you come from an old version, please check out the new simplified CLI syntax
- Note that some commands have suboption (e.g. to skip checking existing hashes, see `chkbit update -h`)

