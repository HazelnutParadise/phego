<?php
// W-17 differential oracle: runs every cases/*.php in pinned PHP 8.5.12 and
// records the exact byte streams. Oracle only, never product execution.
//
// Logging: with log_errors=1 and an empty error_log, the CLI SAPI writes log
// lines to stderr as bare "PHP Warning:  ..." lines without a timestamp, so no
// extra flag is needed (checked against the pinned images).
//
// A case may override an INI value for every mode with comment lines right
// after its opening tag, one setting each: "// ini: error_reporting=30717".
// They are passed as -d after the mode's flags, so they take precedence.
$common = ['-d', 'html_errors=0', '-d', 'error_reporting=-1'];

function ini_overrides(string $path): array
{
    $flags = [];
    $lines = file($path, FILE_IGNORE_NEW_LINES);
    for ($i = 1; $i < count($lines); $i++) {
        if (!preg_match('~^// ini: ([a-z_]+)=(\S+)$~', $lines[$i], $m)) {
            break;
        }
        $flags[] = '-d';
        $flags[] = $m[1] . '=' . $m[2];
    }
    return $flags;
}
$modes = [
    'display' => ['-d', 'display_errors=1', '-d', 'log_errors=0'],
    'log' => ['-d', 'display_errors=0', '-d', 'log_errors=1', '-d', 'error_log='],
    'both' => ['-d', 'display_errors=1', '-d', 'log_errors=1', '-d', 'error_log='],
];

function run_php(array $args): array
{
    $out = '/tmp/w17-stdout';
    $err = '/tmp/w17-stderr';
    $spec = [0 => ['pipe', 'r'], 1 => ['file', $out, 'w'], 2 => ['file', $err, 'w']];
    $p = proc_open(array_merge([PHP_BINARY, '-n'], $args), $spec, $pipes, '/w', []);
    if (!is_resource($p)) {
        throw new RuntimeException('cannot start reference PHP');
    }
    // stdin is closed before the script starts reading.
    fclose($pipes[0]);
    $code = proc_close($p);
    $result = ['stdout' => file_get_contents($out), 'stderr' => file_get_contents($err), 'exit_code' => $code];
    unlink($out);
    unlink($err);
    return $result;
}

$paths = glob('/w/cases/*.php');
sort($paths, SORT_STRING);
$records = [];
foreach ($paths as $path) {
    $name = basename($path, '.php');
    $record = ['case' => $name, 'source_sha256' => hash_file('sha256', $path)];
    $overrides = ini_overrides($path);
    foreach ($modes as $mode => $flags) {
        $r = run_php(array_merge($flags, $common, $overrides, [$path]));
        $record[$mode] = ['stdout_b64' => base64_encode($r['stdout']),
            'stderr_b64' => base64_encode($r['stderr']), 'exit_code' => $r['exit_code']];
    }
    $record['return_json'] = null;
    if (str_starts_with($name, 'return_')) {
        // The include value goes to stderr so the script's own output cannot mix with it.
        $code = 'fwrite(STDERR, json_encode(include $argv[1], JSON_PRESERVE_ZERO_FRACTION | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR));';
        $r = run_php(array_merge(['-d', 'display_errors=0', '-d', 'log_errors=0'], $common, ['-r', $code, '--', $path]));
        if ($r['exit_code'] !== 0) {
            throw new RuntimeException("return helper failed for $name");
        }
        $record['return_json'] = $r['stderr'];
    }
    $records[] = $record;
}
echo json_encode(['php_version' => PHP_VERSION, 'int_size' => PHP_INT_SIZE, 'os' => PHP_OS_FAMILY,
    'machine' => php_uname('m'), 'cases' => $records], JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES), "\n";
