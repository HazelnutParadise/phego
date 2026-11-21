<?php
declare(strict_types=1);
function age(int $years)
{
    return $years;
}
echo "age ", age(10), "\n";
echo age("10"), "\n";
