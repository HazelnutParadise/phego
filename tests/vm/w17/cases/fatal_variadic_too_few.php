<?php
function needs_two($a, $b, ...$rest) {
    echo "not reached\n";
}
function caller() {
    echo "calling\n";
    needs_two(
        1
    );
}
caller();
