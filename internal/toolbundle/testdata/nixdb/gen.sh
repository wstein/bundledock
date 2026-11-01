#!/bin/sh
# Regenerates the two fixtures: the Nix 2.x schema, 120 store paths in the same
# order, WAL mode (closed, so the WAL is checkpointed), different registration
# times (4-byte integers). Usage: gen.sh OUTPUT_DIR (needs sqlite3).
set -eu
out=$1
mkdir -p "$out"
make() {
	db=$1 base=$2 step=$3
	rm -f "$db" "$db-wal" "$db-shm"
	{
		echo 'PRAGMA journal_mode=wal;'
		echo 'create table ValidPaths (id integer primary key autoincrement not null, path text unique not null, hash text not null, registrationTime integer not null, deriver text, narSize integer, ultimate integer, sigs text, ca text);'
		echo 'create table Refs (referrer integer not null, reference integer not null, primary key (referrer, reference), foreign key (referrer) references ValidPaths(id) on delete cascade, foreign key (reference) references ValidPaths(id) on delete restrict);'
		echo 'create index IndexReferrer on Refs(referrer);'
		echo 'create index IndexReference on Refs(reference);'
		echo 'create table DerivationOutputs (drv integer not null, id text not null, path text not null, primary key (drv, id), foreign key (drv) references ValidPaths(id) on delete cascade);'
		i=1
		while [ $i -le 120 ]; do
			n=$(printf '%03d' $i)
			echo "insert into ValidPaths (path, hash, registrationTime, narSize, ultimate, sigs) values ('/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaa$n-package-$n-1.0', 'sha256:0000000000000000000000000000000000000000000000000000000000$n', $((base + i * step)), $((i * 4096)), 0, 'cache.nixos.org-1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef$n==');"
			[ $i -gt 1 ] && echo "insert into Refs values ($i, $((i - 1)));"
			i=$((i + 1))
		done
	} | sqlite3 "$db" >/dev/null
}
make "$out/nixdb-a.sqlite" 1700000000 1
make "$out/nixdb-b.sqlite" 1780000000 7
