<?php
class D { function __destruct() { echo "D\n"; } }
function f($a) { nope(); }
f([new D]);
