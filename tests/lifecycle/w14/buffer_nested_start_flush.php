<?php
// REQ-020/REQ-040; T-079/T-080. The same fatal from an explicit flush stops
// the script; shutdown functions still run after every buffer is discarded.
require __DIR__ . '/common.php';
ob_start(function ($s) { event('flush:nested'); ob_start(); return $s; });
echo 'body';
ob_end_flush();
event('body:after');
