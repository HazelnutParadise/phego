<?php
echo PHP_INT_SIZE, " ", PHP_INT_MAX, " ", PHP_INT_MIN, "\n";
echo PHP_FLOAT_EPSILON, " ", PHP_FLOAT_MAX, " ", PHP_FLOAT_MIN, " ", PHP_FLOAT_DIG, "\n";
echo PHP_VERSION, " ", PHP_MAJOR_VERSION, ".", PHP_MINOR_VERSION, ".", PHP_RELEASE_VERSION, " ", PHP_VERSION_ID, " [", PHP_EXTRA_VERSION, "]\n";
echo PHP_OS, " ", PHP_OS_FAMILY, " ", DIRECTORY_SEPARATOR, " ", PATH_SEPARATOR, " ", PHP_MAXPATHLEN, "\n";
echo PHP_EOL === "\n" ? "lf" : "other", PHP_EOL;
echo E_ERROR, " ", E_WARNING, " ", E_PARSE, " ", E_NOTICE, " ", E_DEPRECATED, " ", E_USER_DEPRECATED, " ", E_ALL, "\n";
echo M_PI, " ", M_E, " ", M_SQRT2, " ", M_LN2, "\n";
echo INF, " ", -INF, " ", NAN, " ", INF > PHP_FLOAT_MAX ? "inf" : "fin", "\n";
echo PHP_DEBUG, PHP_ZTS, " ", PHP_BINARY, " ", PHP_SAPI, "\n";
echo SORT_STRING, " ", COUNT_RECURSIVE, " ", JSON_PRETTY_PRINT, " ", PREG_SPLIT_NO_EMPTY, " ", LC_ALL, " ", DATE_ATOM, "\n";
echo ENT_QUOTES, " ", STR_PAD_LEFT, " ", PHP_ROUND_HALF_UP, " ", M_PI . "", "\n";
echo PHP_INT_MAX + 1, " ", PHP_INT_MIN - 1, " ", PHP_INT_MAX % 7, " ", PHP_INT_SIZE * 8, "\n";
echo PHP_INT_MAX === 9223372036854775807 ? "64-bit" : "32-bit", "\n";
echo Dom\INDEX_SIZE_ERR, " ", \Dom\HIERARCHY_REQUEST_ERR, " ", \dom\NOT_FOUND_ERR, "\n";
