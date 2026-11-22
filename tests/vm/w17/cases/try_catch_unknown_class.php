<?php
// try_catch_unknown_class: catch (NoSuchClass $e) is not an error; the exception passes to the next clause or the outer try
function go($n)
{
    if ($n === 1) {
        throw new RuntimeException("runtime");
    }
    return "none";
}

try {
    throw new RuntimeException("A");
} catch (NoSuchClassX $e) {
    echo "wrong: unknown class clause\n";
} catch (RuntimeException $e) {
    echo "next clause got: ", $e->getMessage(), "\n";
}

try {
    try {
        go(1);
    } catch (NoSuchClassY | AlsoMissing $e) {
        echo "wrong: unknown union\n";
    }
} catch (RuntimeException $e) {
    echo "outer try got: ", $e->getMessage(), "\n";
}

$r = go(0);
echo "no throw result: ", $r, "\n";

try {
    throw new LogicException("L");
} catch (Gone $e) {
    echo "wrong: Gone\n";
} catch (LogicException $e) {
    echo "after unknown clause: ", $e->getMessage(), "\n";
}
