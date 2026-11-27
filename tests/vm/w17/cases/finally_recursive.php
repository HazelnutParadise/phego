<?php
// finally_recursive: recursive function with try/finally at each level; the exception thrown at the bottom unwinds the finally blocks in order
class Deep extends Exception {}

function descend($n)
{
    try {
        echo "enter ", $n, "\n";
        if ($n === 5) {
            throw new Deep("bottom " . $n);
        }
        descend($n + 1);
        echo "returned to ", $n, "\n";
    } finally {
        echo "leave ", $n, "\n";
    }
}

try {
    descend(1);
} catch (Deep $e) {
    echo "caught: ", $e->getMessage(), "\n";
}

function guarded($n)
{
    try {
        if ($n === 0) {
            throw new Deep("zero");
        }
        guarded($n - 1);
        echo "guarded ok ", $n, "\n";
    } catch (Deep $e) {
        echo "guarded catch at ", $n, "\n";
        return;
    } finally {
        echo "guarded finally ", $n, "\n";
    }
    echo "guarded end ", $n, "\n";
}
guarded(2);
echo "after guarded\n";
