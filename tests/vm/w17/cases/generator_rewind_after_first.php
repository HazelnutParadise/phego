<?php
function gen() { echo "body\n"; yield 1; yield 2; }
$g = gen();
foreach ($g as $v) { echo $v, "\n"; break; }
foreach ($g as $v) { echo "again $v\n"; break; }
$g->next();
try { foreach ($g as $v) {} } catch (Exception $e) { echo $e->getMessage(), "\n"; }
