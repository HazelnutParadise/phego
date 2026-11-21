<?php
echo "before\n";
$f = fn() => parent::class;
$f();
