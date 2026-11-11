<?php
// probe.php MODE PATH [read|write]: open PATH like a PHP script would and
// report each result with its time in seconds since the start. MODE is an
// fopen mode, "include" or "fgc" (file_get_contents).
$t0 = microtime(true);
function t() { global $t0; return sprintf("%.1f", microtime(true) - $t0); }
[$_, $op, $path] = $argv + [null, null, null];
switch ($op) {
case 'include':
    $r = include $path;
    echo "include returned ", var_export($r, true), " at ", t(), "\n";
    break;
case 'fgc':
    $r = file_get_contents($path);
    echo "fgc returned ", var_export($r, true), " at ", t(), "\n";
    break;
default:
    $f = fopen($path, $op);
    echo "fopen returned ", $f === false ? 'false' : get_resource_type($f), " at ", t(), "\n";
    if ($f && ($argv[3] ?? '') === 'read') {
        $d = fread($f, 100);
        echo "fread ", var_export($d, true), " at ", t(), " eof=", var_export(feof($f), true), "\n";
        $d = fread($f, 100);
        echo "fread2 ", var_export($d, true), " at ", t(), " eof=", var_export(feof($f), true), "\n";
    }
    if ($f && ($argv[3] ?? '') === 'write') {
        $n = fwrite($f, "payload");
        echo "fwrite ", var_export($n, true), " at ", t(), "\n";
        sleep(1);
        $n = fwrite($f, "again");
        echo "fwrite2 ", var_export($n, true), " at ", t(), "\n";
    }
}
