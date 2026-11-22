<?php
// exception_file_line_creation: getLine and getFile report the creation site, not the throw site
function build_error($msg) {
    $e = new Exception($msg);
    return $e;
}

function throw_it_elsewhere($e) {
    echo "about to throw\n";
    throw $e;
}

$made = build_error("made in helper");
echo "created_line: ", $made->getLine(), "\n";
echo "created_file_is_this: ", ($made->getFile() === __FILE__ ? 1 : 0), "\n";

try {
    throw_it_elsewhere($made);
} catch (Exception $caught) {
    echo "caught_message: ", $caught->getMessage(), "\n";
    echo "caught_line_is_creation: ", $caught->getLine(), "\n";
    echo "caught_same_object: ", ($caught === $made ? 1 : 0), "\n";
}

$x = new Exception("same line"); $expect = __LINE__;
echo "same_line_marker: ", ($x->getLine() === $expect ? 1 : 0), "\n";

function far_away() {
    return new LogicException("far");
}
$far = far_away();
echo "far_line_inside_function: ", $far->getLine(), "\n";
echo "throw_line_not_used: ", ($far->getLine() === __LINE__ ? 1 : 0), "\n";

try {
    throw $far;
} catch (LogicException $l) {
    echo "logic_line_unchanged: ", $l->getLine(), "\n";
}
echo "end\n";
