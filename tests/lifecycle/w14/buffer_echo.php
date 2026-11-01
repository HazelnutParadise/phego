<?php
// REQ-040; T-079/T-080. Handler echo is discarded and deprecated in PHP 8.5.12.
require __DIR__ . '/common.php';
ob_start(function ($s) { event('flush:echo'); echo 'discarded'; return $s; });
echo 'B';
