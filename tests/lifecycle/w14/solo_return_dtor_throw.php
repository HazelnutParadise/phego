<?php
// REQ-040; T-079/T-080. A local destructor that throws after the handler
// returned does not undo the return: PHP 8.5.12 prints the returned string
// and reports the handler-output deprecation before the exception.
function event(string $s): void { fwrite(STDERR, $s . "\n"); }
class T { public function __destruct() { event("destroy:local"); throw new Exception("dtor"); } }
ob_start(function ($s) { event("flush:outer"); return "<" . $s . ">"; });
ob_start(function ($s) { event("flush:inner"); $t = new T; echo "x"; return "R"; });
echo "body";
try { ob_end_flush(); } catch (Exception $e) { event("caught"); echo "caught:", $e->getMessage(); }
echo "|after";
