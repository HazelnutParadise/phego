<?php
// fatal_throw_uncaught_error_exception: uncaught ErrorException with severity and overridden file and line
$e = new ErrorException("custom failure", 3, E_WARNING, "/virtual/override.php", 42);
echo "message: ", $e->getMessage(), "\n";
echo "severity: ", $e->getSeverity(), "\n";
echo "severity_is_warning: ", $e->getSeverity() === E_WARNING ? "yes" : "no", "\n";
echo "file: ", $e->getFile(), "\n";
echo "line: ", $e->getLine(), "\n";
$plain = new ErrorException("defaults");
echo "default_severity: ", $plain->getSeverity(), "\n";
echo "default_line_is_creation: ", $plain->getLine(), "\n";
echo "file_default_is_script: ", $plain->getFile(), "\n";
echo "start\n";
throw $e;
echo "never\n";
