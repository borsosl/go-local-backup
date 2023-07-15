# Local Backup CLI application based on plain text configurations

This CLI program (and importable Go module) reads a list of paths of source files
and directories from config and copies them under a target directory, keeping
all parent folders from the source structure, not including windows volume letter.

The configuration allows the user to cherry-pick certain file paths and types
and sizes from directory hierarchies, fine-tuning what to store and manage the
space used by the backup. This makes it ideal for copying to folders that
automatically sync with some cloud storage provider. The module can also be
used to copy files in bulk, after the configuration is generated by a caller
package.

Copying keeps modification timestamps of the source files, and subsequent
backups sync newer files only to the existing target. In the functional sense
it's incremental backup, but the increments are not separate patches. So
only the latest versions are available, but storage space is spared, and
versions may be retained on the cloud.

The destination permissions will be read-writable files, and listable directories
for current user and group. Deleted source files are not detected in the target
folder and remain there. Symbolic links in the sources are not followed.

The program is used by the author on Windows on a daily basis, but should work
on other systems. The config must use the platform-specific path separator. The
user running the program should have permissions to list and copy specified
source content.

## CLI interface

Build or install:
```
go build|install -ldflags "-s" ./cmd/backup
```

Run:
```
backup <config-file> [--dry-run]
```

## Module function

```go
func Backup(config []string, out io.Writer, dryRun bool) error
```

## Setting the target folder

In the configuration, before any source paths, the mandatory setting is the
target folder. It is written on a separate line with the prefix =>.

```
=> c:\temp\autobackup
```

A target folder is in effect until another one is declared.

## Source files and directories

Lines that only contain file system paths, and have no prefixes (for target
and filters), are considered sources. Single files are just copied as one
entity, directories are recursed, but both can be prevented or limited by
filters.

## Filtering

Files and entire subdirectory structures that are found in a source directory can
be skipped by filters. Each filter is a separate line with an identifying prefix
and the setting. Whitespace is permitted around prefixes. All filters are
optional, and are in effect until overridden by the next filter of the same kind.

- !@ <days int> - max age filter
- !> <size int64> - max size filter
- ! <regexp-list> - exclude filter
- !+ <regexp-list> - extend previous exclude rule-set

### Max age filter

At the time of parsing this line, N days are subtracted from the current time,
then the start of that day (00:00 hours local time) will be the minimum date
of files that are backed up. Default start date is epoch of AD 1, that is all
files on most systems.

```
!@ 30
```

### Max size filter

Only files with the specified maximum size are copied. Default is unlimited.

```
!> 100000000
```

### Exclude filter

Provide a list of regular expressions with patterns that exist in the full
path of files or directories to be skipped from copying or recursing. Separate
patterns with double-comma. A new such filter line deletes previously used exclude
patterns. A single ! will clear all the current patterns without introducing
new ones.

```
!\\node_modules\\,,\\.idea\\,,\.class$,,\\frontend\\build\\
```

Directories are marked with starting and ending slashes, file endings with
a regexp $ sign. When parts of a directory are wanted, either exclude the
unwanted parts as for 'frontend' above, or exclude it all, and then specify
other source lines for the wanted parts.

If there is an error in a pattern, it is ignored at the time and reported
after all sources are recursed. Use dry-run to make sure your patterns are
correct before making the backup with potentially unfiltered items.

### Extend exclude filter

Keeps the previous exclude patterns, and adds additional ones.

```
!+\.jar$,,\.exe$
```

## Comments and empty lines

Empty lines are skipped and lines starting with # are treated as comments.

## Dry-run

When run with this setting, no files are copied, but all the source paths
that would be copied are printed to output instead. Note that the directory
stats in the output shows 'Copied', indicating the would-be copied files, but
they are not actually copied.

## Disclaimer

This program comes with ABSOLUTELY NO WARRANTY. It is the responsibility of
the user to test any (changes in) configuration after making a copy of all
source and destination directories by some other means. This is free software
and the developer shall not be responsible for any data loss.

## Example configuration

```
=>c:\backup\auto\
!@10

!\\_work\\done\\,,\\Thumbnails\\
c:\comics\
c:\Users\Laci\AppData\Local\Google\Chrome\User Data\Default\Bookmarks
c:\Users\Laci\AppData\Roaming\Thunderbird\profiles.ini

!\\prg\\Java\\,,\\node_modules\\,,\.class$,,\.jar$,,\\frontend\\build\\
c:\wk\

=>c:\backup\OneDrive\sync\
!@10000

!\\.indd$,,\\\.git\\,,\\dist\\,,\\input\\yaml\\
c:\wk\memo\
c:\Users\Laci\Documents\
```
