<?php
function pick($v, $extra)
{
    return match ($v) { "a" => 1, "b" => 2 };
}
echo pick("a", [1]), "\n";
echo pick("it's long enough to cut", 2.5);
