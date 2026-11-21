<?php
function plain() { return Closure::getCurrent(); }
echo "before\n";
$g = plain(...);
$g();
