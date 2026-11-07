<?php
// REQ-020/REQ-040; T-079/T-080. ob_start() inside a handler is fatal, and PHP
// discards every buffer at once instead of flushing the outer handlers.
require __DIR__ . '/common.php';
ob_start(function ($s) { event('flush:nested'); ob_start(); return $s; });
echo 'body';
