<?php
// exception_error_exception: ErrorException constructor arguments, severity, file and line overrides and defaults
$prev = new Exception("previous one");
$full = new ErrorException("full message", 3, E_WARNING, "/virtual/file.php", 42, $prev);
echo "full_message: ", $full->getMessage(), "\n";
echo "full_code: ", $full->getCode(), "\n";
echo "full_severity: ", $full->getSeverity(), "\n";
echo "full_file: ", $full->getFile(), "\n";
echo "full_line: ", $full->getLine(), "\n";
echo "full_previous_msg: ", $full->getPrevious()->getMessage(), "\n";
echo "full_previous_is_prev: ", ($full->getPrevious() === $prev ? 1 : 0), "\n";

$here = new ErrorException("defaults");
echo "default_message: ", $here->getMessage(), "\n";
echo "default_code: ", $here->getCode(), "\n";
echo "default_severity_is_error: ", ($here->getSeverity() === E_ERROR ? 1 : 0), "\n";
echo "default_file_is_this: ", ($here->getFile() === __FILE__ ? 1 : 0), "\n";
echo "default_line_is_here: ", ($here->getLine() === __LINE__ - 5 ? 1 : 0), "\n";
echo "default_previous_null: ", ($here->getPrevious() === null ? 1 : 0), "\n";

$nulls = new ErrorException("nulls", 0, E_NOTICE, null, null);
echo "nulls_severity: ", $nulls->getSeverity(), "\n";
echo "nulls_file_is_this: ", ($nulls->getFile() === __FILE__ ? 1 : 0), "\n";
echo "nulls_line_positive: ", ($nulls->getLine() > 0 ? 1 : 0), "\n";

$sev = new ErrorException("sev", 0, E_USER_DEPRECATED, "/a.php", 5);
echo "sev_value: ", $sev->getSeverity(), "\n";
echo "sev_is_exception: ", ($sev instanceof Exception ? 1 : 0), "\n";
echo "sev_file_line: ", $sev->getFile(), ":", $sev->getLine(), "\n";

$only_file = new ErrorException("file only", 0, E_WARNING, "/b.php");
echo "only_file_line_when_omitted: ", $only_file->getLine(), "\n";
echo "only_file_file: ", $only_file->getFile(), "\n";
