<?php
// Runs an effects fixture and prints, per snippet line, the events PHP
// observed: "call" when user code ran (magic methods, autoloaders,
// destructors, iterators, closures), "warn" for notices, warnings and
// deprecations, "throw" for exceptions. Run in the pinned PHP 8.5.12 image:
//   php -n effects.php /w/effects/magic.php
// TestEffectsCoverObservedBehaviour in internal/ir checks that HIR claims
// every observed effect.
$fixture = $argv[1];
$start = 0;
foreach (file($fixture) as $i => $line) {
    if (str_contains($line, '// --- snippets ---')) {
        $start = $i + 2;
    }
}
$events = [];
// The outermost fixture frame on a snippet line is the statement that
// caused the event.
function snippet_line(array $trace, ?int $line = null): ?int {
    global $fixture, $start;
    $found = ($line !== null && $line >= $start) ? $line : null;
    foreach ($trace as $frame) {
        if (($frame['file'] ?? null) === $fixture && ($frame['line'] ?? 0) >= $start) {
            $found = $frame['line'];
        }
    }
    return $found;
}
function record(string $kind, ?int $line): void {
    global $events;
    if ($line !== null && !in_array($kind, $events[$line] ?? [], true)) {
        $events[$line][] = $kind;
    }
}
function thrown(int $line): void { record('throw', $line); }
set_error_handler(function ($no, $msg, $file, $line) use (&$fixture) {
    record('warn', snippet_line(debug_backtrace(), $file === $fixture ? $line : null));
    return true;
});
// Fixture functions call entered() first, so a snippet that runs them
// records a call.
function entered(): void { record('call', snippet_line(debug_backtrace())); }
ob_start();
include $fixture;
ob_end_clean();
ksort($events);
echo json_encode((object)$events, JSON_PRETTY_PRINT), "\n";
