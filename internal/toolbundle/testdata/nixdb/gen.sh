#!/bin/sh
# Regenerates the fixtures. Usage: gen.sh OUTPUT_DIR (needs the sqlite3 shell).
#
# schema.sql is NixOS/nix src/libstore/schema.sql, byte for byte, from commit
# 5286c0477d77fb919df890324d3a510a199f450e (git blob 77616cd6231f...; the last
# commit that changed the file on 2026-03-10). It carries SQL comments. Which
# version of this schema the Nix inside the pinned nixos/nix image uses is not
# checked. Nix creates its database in WAL mode, so the fixtures do too (closed,
# so the WAL is checkpointed). secure_delete is off in every fixture, so freed
# space keeps its old bytes, as in an SQLite built without SQLITE_SECURE_DELETE.
# The sqlite3 shell of the generating machine writes the file; its page reserve
# and library version are not Nix's.
#
#   nixdb-a, nixdb-b   120 paths, same order, registration times from 1700000001
#                      and from 1780000007 (4-byte integers)
#   nixdb-split        600 paths: the table b-tree root has split
#   nixdb-update       600 paths, then rows updated and deleted
#   nixdb-overflow     a row whose sigs spill onto overflow pages
#   nixdb-widths       synthetic table: registrationTime as the constant 1 and as
#                      1, 2, 3, 4 and 6 byte integers, in the third column
#   nixdb-late         synthetic table: registrationTime behind a 6000 byte
#                      column, i.e. on an overflow page
set -eu
out=$1
here=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$out"

make() { # name rows base step [sigs expression]
	name=$1 rows=$2 base=$3 step=$4 sigs=${5:-"printf('cache.nixos.org-1:%0100d==', i)"}
	db=$out/$name.sqlite
	rm -f "$db" "$db-wal" "$db-shm"
	sqlite3 "$db" >/dev/null <<SQL
PRAGMA journal_mode=wal;
PRAGMA secure_delete=0;
.read $here/schema.sql
with recursive n(i) as (select 1 union all select i+1 from n where i<$rows)
insert into ValidPaths (path, hash, registrationTime, narSize, ultimate, sigs)
  select printf('/nix/store/%032d-package-%d-1.0', i, i), printf('sha256:%064d', i), $base + i * $step, i * 4096, 0, $sigs from n;
with recursive n(i) as (select 2 union all select i+1 from n where i<$rows)
insert into Refs select i, i-1 from n;
SQL
	rm -f "$db-wal" "$db-shm"
}

make nixdb-a 120 1700000000 1
make nixdb-b 120 1780000000 7
make nixdb-split 600 1700000000 3

make nixdb-update 600 1700000000 3
sqlite3 "$out/nixdb-update.sqlite" >/dev/null <<'SQL'
PRAGMA secure_delete=0;
update ValidPaths set registrationTime = registrationTime + 1000000, sigs = sigs || sigs where id % 7 = 0;
delete from ValidPaths where id % 11 = 0;
SQL
rm -f "$out/nixdb-update.sqlite-wal" "$out/nixdb-update.sqlite-shm"

make nixdb-overflow 10 1700000000 5 "printf('%06000d', i)"

rm -f "$out/nixdb-widths.sqlite"
sqlite3 "$out/nixdb-widths.sqlite" >/dev/null <<'SQL'
PRAGMA journal_mode=wal;
create table ValidPaths (id integer primary key autoincrement not null, path text unique not null, registrationTime integer not null, hash text not null);
insert into ValidPaths (path, registrationTime, hash) values ('/nix/store/a-1', 1, 'h1'), ('/nix/store/b-2', 100, 'h2'), ('/nix/store/c-3', 300, 'h3'), ('/nix/store/d-4', 70000, 'h4'), ('/nix/store/e-5', 1700000000, 'h5'), ('/nix/store/f-6', 5000000000, 'h6');
SQL

rm -f "$out/nixdb-late.sqlite"
sqlite3 "$out/nixdb-late.sqlite" >/dev/null <<'SQL'
PRAGMA journal_mode=wal;
create table ValidPaths (id integer primary key, big text, registrationTime integer);
insert into ValidPaths values (1, printf('%06000d', 1), 1700000001);
SQL
rm -f "$out"/*-wal "$out"/*-shm
