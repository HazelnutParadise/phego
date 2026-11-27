<?php
// try_empty_blocks: empty try with catch, try with empty catch swallowing an exception, nested empty blocks
class Quiet extends Exception {}

function bang($n)
{
    throw new Quiet("bang " . $n);
}

echo "0 e before any catch: ", isset($e) ? "yes" : "no", "\n";

echo "1 empty try\n";
try {
} catch (Quiet $e) {
    echo "never\n";
}
echo "1 done\n";

echo "2 empty catch swallows\n";
try {
    bang(1);
} catch (Quiet $e) {
}
echo "2 e message after swallow: ", $e->getMessage(), "\n";

echo "3 nested empty\n";
try {
    try {
    } catch (Quiet $e) {
    }
    try {
        bang(2);
    } catch (Quiet $e) {
    }
} catch (Exception $e) {
    echo "never\n";
}
echo "3 e message: ", $e->getMessage(), "\n";

echo "4 empty catch in a loop\n";
for ($i = 1; $i <= 3; $i++) {
    try {
        if ($i !== 2) {
            bang($i);
        }
    } catch (Quiet $e) {
    }
    echo "4 iteration ", $i, "\n";
}
echo "end\n";
