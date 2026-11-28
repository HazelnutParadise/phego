<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
switch (new D(1)) { default: echo "body\n"; }
echo "after1\n";
function f() {
    try {
        switch (new D(2)) { case 1: echo "one\n"; default: echo "def\n"; break; }
        echo "after-switch\n";
    } finally { echo "fin\n"; }
}
f();
function g() {
    switch (new D(3)) {
        default:
            try { echo "in\n"; break; } finally { echo "fin-g\n"; }
    }
    echo "after-g\n";
}
g();
