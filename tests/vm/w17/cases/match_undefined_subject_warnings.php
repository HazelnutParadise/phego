<?php
echo match ($u1) { 1 => 1, 2 => 2, default => "jumptable" }, "\n";
echo match ($u2) { 1 => 1, $y => "chain" }, "\n";
echo match ($u3) { "a" => 1, 2 => 2, 3 => 3, default => "jumptable-mixed" }, "\n";
echo match ($u4) { 1.5 => 1, 2 => 2, 3 => 3, default => "chain-float" }, "\n";
echo match ($u5) { null => "n", 0 => "z" }, "\n";
echo match ($u6) { default => "only-default" }, "\n";
echo match ($u7) { 1 => "single" , default => "single-chain" }, "\n";
echo match ($u8) { true => "t", false => "f", default => "chain-bool" }, "\n";
echo match (null) { null => "literal null" }, "\n";
echo "end\n";
