<?php
function inner() { yield 'a'; yield 'b'; yield 'k' => 'c'; }
function outer() { yield 'x'; yield from inner(); yield 'y'; yield from ['p', 'q']; yield 'z'; }
foreach (outer() as $k => $v) echo "$k => $v\n";
