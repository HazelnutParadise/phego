<?php
echo "never printed\n";
function f(...$a) {
}
f(...[1, 2],
    3);
