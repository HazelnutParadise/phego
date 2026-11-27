<?php
// fatal_throw_uncaught_empty_message: a code-only exception is inspected, then an uncaught new Exception() with no message ends the script
$coded = new Exception("", 4);
echo "code: ", $coded->getCode(), "\n";
echo "coded_line: ", $coded->getLine(), "\n";
echo "message_empty: ", $coded->getMessage() === "" ? "yes" : "no", "\n";
$bare = new Exception();
echo "bare_code: ", $bare->getCode(), "\n";
echo "bare_line: ", $bare->getLine(), "\n";
echo "bare_file_is_script: ", $bare->getFile(), "\n";
echo "bare_message_empty: ", $bare->getMessage() === "" ? "yes" : "no", "\n";
echo "bare_previous_null: ", $bare->getPrevious() === null ? "yes" : "no", "\n";
echo "about_to_throw\n";
throw new Exception();
echo "never\n";
