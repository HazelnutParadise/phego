<?php
function walk(iterable $it)
{
    foreach ($it as $v) {
        echo $v;
    }
    echo "\n";
}
walk([1, 2, 3]);
walk(5);
