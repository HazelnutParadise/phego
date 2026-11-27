<?php
// try_catch_union: catch (A | B $e) with a class matching neither falling to the next clause; three-way union
class NetError extends RuntimeException {}
class DiskError extends RuntimeException {}
class ParseErr extends LogicException {}

function job($n)
{
    if ($n === 1) {
        throw new NetError("net");
    }
    if ($n === 2) {
        throw new DiskError("disk");
    }
    if ($n === 3) {
        throw new ParseErr("parse");
    }
    if ($n === 4) {
        throw new DomainException("domain");
    }
    if ($n === 5) {
        throw new Error("engine");
    }
    return "fine";
}

for ($i = 1; $i <= 6; $i++) {
    try {
        $r = job($i);
        echo "job ", $i, ": ", $r, "\n";
    } catch (NetError | DiskError $e) {
        echo "job ", $i, " two-way union: ", $e::class, " ", $e->getMessage(), "\n";
    } catch (ParseErr | DomainException | Error $e) {
        echo "job ", $i, " three-way union: ", $e::class, " ", $e->getMessage(), "\n";
    }
}
echo "loop finished at i=", $i, "\n";
