# Upload scan compared with PHP

Back to [`../../../README.md`](../../../README.md) ("Upload scan").

Checks the plugin against a real parser: Traefik with the plugin from this checkout, in front of
PHP 8.3, asking a stand-in clamd that records the SHA-256 of every stream it gets. **Every
non-empty file PHP stores must have reached clamd byte for byte**, and an EICAR file PHP stores
must be logged `infected`. Byte for byte, because clamd also detects files by a hash over the
whole file: an extra line break or a cut file would slip past it, which the EICAR string - only
checked at a file's start - does not show. Repeat it before blocking (phase 3) and after every
change to how the plugin cuts multipart bodies.

```shell
./run.sh                          # crafted uploads, then 4 x 400 random bodies
SEEDS="1 2 3" COUNT=1000 ./run.sh # other or more random bodies
MISSES=/tmp/misses ./run.sh       # keep the bodies that missed, to replay them
```

Needs Docker and Python 3; everything runs on `127.0.0.1:18081` (`PORT`) and is removed
afterwards. It exits 1 on a miss.

- `run.sh` starts the three containers and runs both checks. Traefik is pinned to the version k3s
  ships - keep it in step.
- `fakeclamd.py` speaks clamd's `INSTREAM` and prints each stream's SHA-256; it reports EICAR
  only at the start of a stream, as ClamAV does.
- `index.php` answers with what PHP stored: each upload's size, SHA-256 and whether it holds
  EICAR.

A body the plugin marks `ambiguous` counts as handled even where its bytes did not reach clamd
exactly: blocking refuses it, however a parser reads it. The random run reports how many there
were.

- `crafted.py`: hand-made uploads - boundary tricks, duplicate and odd parameters, file names
  twice, LF and mixed line endings, folded headers, header lines without a colon, preambles,
  parts after the closing delimiter, no closing boundary, quoted-printable, 25 files, raw
  bodies. Small form and JSON bodies without `Content-Disposition` are skipped by the plugin on
  purpose and do not count as misses.
- `random_bodies.py`: random multipart bodies from a seed - random line endings, junk header
  lines, missing or extra delimiters, epilogues, random file bytes. These found two cases the
  crafted ones had not (2026-11-06).

The EICAR test string is built from two halves in every file here, so none holds it whole.
