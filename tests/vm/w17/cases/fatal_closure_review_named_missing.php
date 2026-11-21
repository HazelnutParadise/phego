<?php
echo "before\n";
$f = function () {};
$f->bindTo(newScope: "X");
