<?php
// REQ-020/REQ-040; T-039/T-040/T-079/T-080. Oracle only, never product execution.
$records = [];
foreach (glob('/fixtures/*.php') as $path) {
    $name = basename($path);
    if (in_array($name, ['common.php', 'reference.php'], true)) continue;
    $cmd = [PHP_BINARY, '-n', '-d', 'display_errors=stderr', '-d', 'log_errors=0',
            '-d', 'error_reporting=-1', $path];
    // Files avoid pipe deadlock and preserve the exact byte streams.
    $out = '/tmp/stdout'; $err = '/tmp/stderr';
    $p = proc_open($cmd, [0 => ['file', '/dev/null', 'r'], 1 => ['file', $out, 'w'], 2 => ['file', $err, 'w']], $pipes);
    if (!is_resource($p)) throw new RuntimeException('cannot start reference PHP');
    $code = proc_close($p);
    $stdout = file_get_contents($out); $stderr = file_get_contents($err);
    $p = proc_open($cmd, [0 => ['file', '/dev/null', 'r'], 1 => ['file', '/tmp/combined', 'w'], 2 => ['redirect', 1]], $pipes);
    if (!is_resource($p)) throw new RuntimeException('cannot start merged reference PHP');
    $mergedCode = proc_close($p);
    if ($mergedCode !== $code) throw new RuntimeException('reference runs disagree on exit code');
    // display_errors=1 writes displayed errors through the output layer, into
    // any active buffer and its handler; log_errors=1 with no error_log sends
    // the log copy to stderr. Both channels are recorded.
    $show = [PHP_BINARY, '-n', '-d', 'display_errors=1', '-d', 'log_errors=1',
             '-d', 'html_errors=0', '-d', 'error_reporting=-1', $path];
    $p = proc_open($show, [0 => ['file', '/dev/null', 'r'], 1 => ['file', $out, 'w'], 2 => ['file', $err, 'w']], $pipes);
    if (!is_resource($p)) throw new RuntimeException('cannot start display reference PHP');
    $showCode = proc_close($p);
    $records[] = ['case' => substr($name, 0, -4), 'exit_code' => $code,
        'stdout' => $stdout, 'stderr' => $stderr,
        'combined' => file_get_contents('/tmp/combined'),
        'display' => ['exit_code' => $showCode, 'stdout' => file_get_contents($out),
                      'stderr' => file_get_contents($err)]];
}
echo json_encode(['php_version' => PHP_VERSION, 'int_size' => PHP_INT_SIZE,
                 'os' => PHP_OS_FAMILY, 'cases' => $records], JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT), "\n";
