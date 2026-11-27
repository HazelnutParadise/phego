<?php
function f($x) { foreach ([1] as $q) { switch ($x) { case 1: try { return "one"; } catch (Exception $e) {} } } return "none"; }
echo f(1), f(2), "\n";
