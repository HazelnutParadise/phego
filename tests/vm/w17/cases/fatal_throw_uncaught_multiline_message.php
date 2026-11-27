<?php
// fatal_throw_uncaught_multiline_message: uncaught exception whose message has newlines and indentation
$m = "first line\nsecond line\n  indented third line";
$e = new Exception($m);
echo "message_back: ", $e->getMessage() === $m ? "same" : "diff", "\n";
$lines = 1;
$i = 0;
while (isset($m[$i])) {
    if ($m[$i] === "\n") {
        $lines++;
    }
    $i++;
}
echo "message_lines: ", $lines, "\n";
echo "start\n";
throw $e;
echo "never\n";
