<?php
// throw_rethrow_same_object: rethrowing a caught exception keeps the same object, line, file and trace string
$saved_obj = null;
$saved_line = 0;
$saved_trace = "";

function origin_fail() {
    throw new Exception("origin");
}

function middle_pass() {
    global $saved_obj, $saved_line, $saved_trace;
    try {
        origin_fail();
    } catch (Exception $e) {
        $saved_obj = $e;
        $saved_line = $e->getLine();
        $saved_trace = $e->getTraceAsString();
        echo "middle_catch_line: ", $saved_line, "\n";
        throw $e;
    }
}

try {
    middle_pass();
    echo "not reached\n";
} catch (Exception $outer) {
    echo "outer_message: ", $outer->getMessage(), "\n";
    echo "outer_line_unchanged: ", ($outer->getLine() === $saved_line ? 1 : 0), "\n";
    echo "outer_file_is_origin: ", ($outer->getFile() === __FILE__ ? 1 : 0), "\n";
    echo "outer_same_object: ", ($outer === $saved_obj ? 1 : 0), "\n";
    echo "outer_trace_unchanged: ", ($outer->getTraceAsString() === $saved_trace ? 1 : 0), "\n";
    echo "trace:\n", $outer->getTraceAsString(), "\n";
}

function double_rethrow() {
    try {
        try {
            origin_fail();
        } catch (Exception $inner) {
            throw $inner;
        }
    } catch (Exception $again) {
        throw $again;
    }
}

try {
    double_rethrow();
} catch (Exception $last) {
    echo "double_message: ", $last->getMessage(), "\n";
    echo "double_file_is_origin: ", ($last->getFile() === __FILE__ ? 1 : 0), "\n";
    echo "double_trace:\n", $last->getTraceAsString(), "\n";
}
echo "end\n";
