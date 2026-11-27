<?php
// finally_in_loop_many: 50 iterations of try/finally with continue; the finally block sums values and only totals are printed
$sum = 0;
$evens = 0;
for ($i = 1; $i <= 50; $i++) {
    try {
        if ($i % 5 === 0) {
            continue;
        }
        $sum = $sum + 1;
    } finally {
        $sum = $sum + $i;
        if ($i % 2 === 0) {
            $evens = $evens + 1;
        }
    }
}
echo "iterations: 50\n";
echo "sum from finally: ", $sum, "\n";
echo "even count: ", $evens, "\n";
echo "final i: ", $i, "\n";
