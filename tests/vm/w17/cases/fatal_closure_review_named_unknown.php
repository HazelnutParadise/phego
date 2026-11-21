<?php
function plain($n) { return "p" . $n; }
echo "before\n";
Closure::fromCallable(callable: "plain");
