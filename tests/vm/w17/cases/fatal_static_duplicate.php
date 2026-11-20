<?php
echo "never printed\n";
function f() {
    static $a = 1;
    static $b = 2,
        $a = 3;
}
