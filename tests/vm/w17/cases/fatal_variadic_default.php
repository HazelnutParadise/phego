<?php
echo "never printed\n";
function f($a,
    ...$rest = [1]) {
}
