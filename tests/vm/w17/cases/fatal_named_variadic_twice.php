<?php
function f(...$v) {
    echo "called\n";
}
f(x: 1, y: 2);
f(x: 1, y: 2,
    x: 3);
