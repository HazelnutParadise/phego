<?php
class D { function __construct(int $n) { } function __destruct() { echo "~D\n"; } }
new D("x");
