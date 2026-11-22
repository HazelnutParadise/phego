<?php
// finally_try_inside_finally: an exception raised and caught entirely inside a finally while another is pending; the pending one continues afterwards
class Pend extends Exception {}
class Local extends Exception {}

function pendingKept()
{
    try {
        throw new Pend("pending one");
    } finally {
        echo "finally start\n";
        try {
            throw new Local("local one");
        } catch (Local $l) {
            echo "local caught: ", $l->getMessage(), "\n";
        }
        echo "finally end\n";
    }
    echo "never\n";
}

try {
    pendingKept();
} catch (Pend $p) {
    echo "pending continued: ", $p->getMessage(), "\n";
    echo "pending has previous: ", $p->getPrevious() === null ? "no" : "yes", "\n";
}
echo "end\n";
