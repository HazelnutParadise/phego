<?php
class It implements Iterator {
    function current(): mixed { return 1; } function key(): mixed { return 1; }
    function next(): void {} function rewind(): void {} function valid(): string { return ""; }
}
