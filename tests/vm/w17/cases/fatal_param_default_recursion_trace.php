<?php
function walk($n, $path = "", $limit = 3)
{
    if ($n > $limit) {
        return $path % 0;
    }
    return walk($n + 1, $path . $n);
}
echo walk(1, "", 5), "\n";
echo walk(1), "\n";
echo "not reached\n";
