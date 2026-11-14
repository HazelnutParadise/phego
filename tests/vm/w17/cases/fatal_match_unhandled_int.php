<?php
echo "before\n";
echo match (5) { 1 => "one", 2 => "two" };
echo "never\n";
