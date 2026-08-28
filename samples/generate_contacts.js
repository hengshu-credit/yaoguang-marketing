#!/usr/bin/env node

const fs = require('fs');

// Sample data
const firstNames = [
    'James', 'Mary', 'John', 'Patricia', 'Robert', 'Jennifer', 'Michael', 'Linda',
    'William', 'Elizabeth', 'David', 'Barbara', 'Richard', 'Susan', 'Joseph', 'Jessica',
    'Thomas', 'Sarah', 'Charles', 'Karen', 'Christopher', 'Nancy', 'Daniel', 'Lisa',
    'Matthew', 'Betty', 'Anthony', 'Margaret', 'Mark', 'Sandra', 'Donald', 'Ashley',
    'Steven', 'Kimberly', 'Paul', 'Emily', 'Andrew', 'Donna', 'Joshua', 'Michelle',
    'Kenneth', 'Dorothy', 'Kevin', 'Carol', 'Brian', 'Amanda', 'George', 'Melissa',
    'Edward', 'Deborah', 'Ronald', 'Stephanie', 'Timothy', 'Rebecca', 'Jason', 'Sharon',
    'Jeffrey', 'Laura', 'Ryan', 'Cynthia', 'Jacob', 'Kathleen', 'Gary', 'Amy',
    'Nicholas', 'Angela', 'Eric', 'Shirley', 'Jonathan', 'Anna', 'Stephen', 'Ruth'
];

const lastNames = [
    'Smith', 'Johnson', 'Williams', 'Jones', 'Brown', 'Davis', 'Miller', 'Wilson',
    'Moore', 'Taylor', 'Anderson', 'Thomas', 'Jackson', 'White', 'Harris', 'Martin',
    'Thompson', 'Garcia', 'Martinez', 'Robinson', 'Clark', 'Rodriguez', 'Lewis', 'Lee',
    'Walker', 'Hall', 'Allen', 'Young', 'Hernandez', 'King', 'Wright', 'Lopez',
    'Hill', 'Scott', 'Green', 'Adams', 'Baker', 'Gonzalez', 'Nelson', 'Carter',
    'Mitchell', 'Perez', 'Roberts', 'Turner', 'Phillips', 'Campbell', 'Parker', 'Evans',
    'Edwards', 'Collins', 'Stewart', 'Sanchez', 'Morris', 'Rogers', 'Reed', 'Cook',
    'Morgan', 'Bell', 'Murphy', 'Bailey', 'Rivera', 'Cooper', 'Richardson', 'Cox',
    'Howard', 'Ward', 'Torres', 'Peterson', 'Gray', 'Ramirez', 'James', 'Watson'
];

// Domains for email addresses
const domains = [
    'gmail.com', 'yahoo.com', 'hotmail.com', 'outlook.com', 'icloud.com',
    'aol.com', 'protonmail.com', 'mail.com', 'zoho.com', 'example.com'
];

// Function to get a random element from an array
function getRandomElement(array) {
    return array[Math.floor(Math.random() * array.length)];
}

// Function to generate a random email based on first and last name
function generateEmail(firstName, lastName) {
    const random = Math.random();
    const domain = getRandomElement(domains);

    if (random < 0.33) {
        return `${firstName.toLowerCase()}.${lastName.toLowerCase()}@${domain}`;
    } else if (random < 0.66) {
        return `${firstName.toLowerCase()}${lastName.toLowerCase()}@${domain}`;
    } else {
        return `${firstName.toLowerCase()[0]}${lastName.toLowerCase()}@${domain}`;
    }
}

// Generate the data
const csvHeader = 'first_name,last_name,email\n';
let csvContent = csvHeader;

for (let i = 0; i < 1000; i++) {
    const firstName = getRandomElement(firstNames);
    const lastName = getRandomElement(lastNames);
    const email = generateEmail(firstName, lastName);

    csvContent += `${firstName},${lastName},${email}\n`;
}

// Write to file
fs.writeFileSync('1000_contacts.csv', csvContent);

console.log('Generated 1000 contacts in 1000_contacts.csv'); 