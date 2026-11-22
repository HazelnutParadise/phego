<?php
// CVE: CVE-2010-4645
// zend_strtod looped forever on 2.2250738585072011e-308 because of x87
// extended precision (Zend/tests/bug53632.phpt).
$d = 2.2250738585072011e-308;
echo $d, "\n";
echo "2.2250738585072011e-308" + 0, "\n";
echo (float) "2.2250738585072011e-308", "\n";
echo "0.00022250738585072011e-304" * 1, "\n";
echo 2.2250738585072012e-308, " ", 2.2250738585072014e-308, "\n";
echo $d == "2.2250738585072011e-308" ? "equal\n" : "different\n";
echo "Done\n";
