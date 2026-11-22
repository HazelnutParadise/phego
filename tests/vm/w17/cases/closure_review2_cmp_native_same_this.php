<?php
$f = function () {};
echo $f->bindTo(...) == $f->call(...) ? "eq" : "ne", "\n";
echo $f->bindTo(...) == $f->bindTo(...) ? "eq" : "ne", "\n";
