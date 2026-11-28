<?php
function gen() { yield 1; }
$g = gen(); $g->current();
$g->throw(new DomainException("thrown in"));
