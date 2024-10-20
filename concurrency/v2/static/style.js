        // Dots connected and interactive effect
        const canvas = document.getElementById('canvas');
        const ctx = canvas.getContext('2d');
        const dots = [];
        let mouseX = -100, mouseY = -100;

        window.addEventListener('resize', resizeCanvas);
        resizeCanvas();

        function resizeCanvas() {
            canvas.width = window.innerWidth;
            canvas.height = window.innerHeight;
            createDots();
        }

        function createDots() {
            dots.length = 0;
            for (let i = 0; i < 100; i++) {
                dots.push({
                    x: Math.random() * canvas.width,
                    y: Math.random() * canvas.height,
                    radius: Math.random() * 3 + 2,
                    dx: (Math.random() - 0.5) * 2,
                    dy: (Math.random() - 0.5) * 2
                });
            }
        }

        function moveDots() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);
            dots.forEach(dot => {
                dot.x += dot.dx;
                dot.y += dot.dy;

                // Bounce from edges
                if (dot.x < 0 || dot.x > canvas.width) dot.dx *= -1;
                if (dot.y < 0 || dot.y > canvas.height) dot.dy *= -1;

                // Mouse interaction
                const distX = dot.x - mouseX;
                const distY = dot.y - mouseY;
                const distance = Math.sqrt(distX * distX + distY * distY);
                if (distance < 100) {
                    const angle = Math.atan2(distY, distX);
                    dot.x += Math.cos(angle) * 3;
                    dot.y += Math.sin(angle) * 3;
                }

                // Draw dot
                ctx.beginPath();
                ctx.arc(dot.x, dot.y, dot.radius, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(255, 255, 255, 0.7)';
                ctx.fill();

                // Draw lines between nearby dots
                dots.forEach(otherDot => {
                    const dist = Math.sqrt((dot.x - otherDot.x) ** 2 + (dot.y - otherDot.y) ** 2);
                    if (dist < 100) {
                        ctx.beginPath();
                        ctx.moveTo(dot.x, dot.y);
                        ctx.lineTo(otherDot.x, otherDot.y);
                        ctx.strokeStyle = 'rgba(255, 255, 255, 0.1)';
                        ctx.stroke();
                    }
                });
            });
        }

        canvas.addEventListener('mousemove', (e) => {
            mouseX = e.clientX;
            mouseY = e.clientY;
        });

        setInterval(moveDots, 30);

        document.getElementById('matrixForm').addEventListener('submit', function(e) {
            e.preventDefault();
            
            // Show loading message
            document.getElementById('loading').style.display = 'flex';
            document.getElementById('result').innerText = ''; // Clear previous result

            // Get the matrix size from the input
            const size = document.getElementById('size').value;

            // Make an AJAX request to the server
            fetch(`/cpu-intensive?size=${size}`)
                .then(response => response.text())
                .then(data => {
                    // Hide the loading message and show the result
                    document.getElementById('loading').style.display = 'none';
                    document.getElementById('result').innerText = data;
                })
                .catch(error => {
                    document.getElementById('loading').style.display = 'none';
                    document.getElementById('result').innerText = 'Error occurred: ' + error;
                });
        });

        document.getElementById('matrixForm2').addEventListener('submit', function(e) {
            e.preventDefault();
            
            // Show loading message
            document.getElementById('loading2').style.display = 'flex';
            document.getElementById('result2').innerHTML = ''; // Clear previous result
            document.getElementById('result2').classList.remove('show'); // Remove the show class for fade out
        
            // Make an AJAX request to the server
            fetch(`/all-values`)
                .then(response => response.text())
                .then(data => {
                    // Hide the loading message
                    document.getElementById('loading2').style.display = 'none';
                    document.getElementById('result2').innerHTML = data; // Use innerHTML to render HTML
        
                    // Trigger fade-in effect
                    setTimeout(() => {
                        document.getElementById('result2').classList.add('show'); // Add the show class for fade in
                    }, 10); // Small delay to ensure the previous opacity change is applied
                })
                .catch(error => {
                    document.getElementById('loading2').style.display = 'none';
                    document.getElementById('result2').innerText = 'Error occurred: ' + error;
                });
        });
        
        