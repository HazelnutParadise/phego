<?php
// fatal_throw_uncaught_exception: uncaught new Exception at top level after normal output
echo "step one\n";
echo "step two\n";
$pending = new Exception("boom", 4);
echo "class: ", $pending::class, "\n";
echo "message: ", $pending->getMessage(), "\n";
echo "code: ", $pending->getCode(), "\n";
echo "previous_is_null: ", $pending->getPrevious() === null ? "yes" : "no", "\n";
echo "line_of_creation: ", $pending->getLine(), "\n";
echo "file_is_script: ", $pending->getFile(), "\n";
echo "trace_is_main: ", $pending->getTraceAsString(), "\n";
echo "about_to_throw\n";
throw $pending;
echo "never printed\n";
