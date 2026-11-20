<?php
echo "never printed\n";
function f(...$a) {
}
f(a: 1,
    ...[2]);
