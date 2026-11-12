<?php
function deep($n)
{
    if ($n == 0) {
        echo "exiting\n";
        exit(5);
    }
    echo "level", $n, "\n";
    deep($n - 1);
    echo "never returns\n";
}
deep(2);
echo "never\n";
