<?php
// exception_get_trace_array: foreach over getTrace() frames, printing keys, file, line, function, class, type and scalar args
function source_exc($n, $s) {
    return new Exception("src");
}

class Walker {
    function step($n, $s) {
        return source_exc($n, $s);
    }
    static function go($n) {
        $w = new Walker();
        return $w->step($n, "tail");
    }
}

function outer_fn($x) {
    return Walker::go($x);
}

$e = outer_fn(7);
$frames = $e->getTrace();
$index = 0;
foreach ($frames as $frame) {
    echo "frame_", $index, "_keys:";
    foreach ($frame as $key => $value) {
        echo " ", $key;
    }
    echo "\n";
    echo "frame_", $index, "_function: ", $frame['function'], "\n";
    if (isset($frame['class'])) {
        echo "frame_", $index, "_class: ", $frame['class'], "\n";
    }
    if (isset($frame['type'])) {
        echo "frame_", $index, "_type: ", $frame['type'], "\n";
    }
    if (isset($frame['file'])) {
        echo "frame_", $index, "_file: ", $frame['file'], "\n";
    }
    if (isset($frame['line'])) {
        echo "frame_", $index, "_line: ", $frame['line'], "\n";
    }
    if (isset($frame['args'])) {
        $ai = 0;
        foreach ($frame['args'] as $arg) {
            echo "frame_", $index, "_arg", $ai, ": ", $arg, "\n";
            $ai = $ai + 1;
        }
    }
    $index = $index + 1;
}
echo "frame_total: ", $index, "\n";

$top = new Exception("top");
echo "top_trace_is_empty: ", ($top->getTrace() === [] ? 1 : 0), "\n";
echo "first_frame_is_source: ", $frames[0]['function'], "\n";
echo "class_of_method_frame: ", $frames[1]['class'], $frames[1]['type'], $frames[1]['function'], "\n";
echo "class_of_static_frame: ", $frames[2]['class'], $frames[2]['type'], $frames[2]['function'], "\n";
