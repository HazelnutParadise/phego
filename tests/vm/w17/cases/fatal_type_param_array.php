<?php
function first(array $items)
{
    return $items[0];
}
echo first([1]), "\n";
echo first("not array"), "\n";
