<?php
$f = function () { return func(); };
function func() { return (function () { return (fn() => undefined_fn())(); })(); }
echo "s\n";
$f();
