<?php
class T { function __destruct() { throw new Exception("t"); } }
function f($v) {
    switch (new T) {
        case 1:
            echo "one\n";
            break;
        default:
            echo "def\n";

            echo "last\n";
    }
    echo "after\n";
}
try { f(1); } catch (Exception $e) { echo $e->getTraceAsString(), "\n"; }
function g() {
    foreach ([1, 2] as $i) {
        switch (new T) {
            default:
                echo "in\n";
                continue 2;
        }
    }
}
try { g(); } catch (Exception $e) { echo $e->getTraceAsString(), "\n"; }
