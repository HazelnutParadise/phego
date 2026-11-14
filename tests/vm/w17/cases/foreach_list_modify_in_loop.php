<?php
$rows = [[1, 2], [3, 4]];
foreach ($rows as $k => [$a, $b]) {
    $rows[] = [$a + 10, $b + 10];
    $rows[$k] = 'x';
    echo $k, $a, $b, "\n";
}
echo $rows[0], $rows[1], $rows[2][0], $rows[3][1], "\n";
$refRows = [[1], [2]];
foreach ($refRows as [&$v]) {
    if ($v === 1) {
        $refRows[] = [3];
    }
    $v *= 2;
}
unset($v);
echo $refRows[0][0], $refRows[1][0], $refRows[2][0], "\n";
